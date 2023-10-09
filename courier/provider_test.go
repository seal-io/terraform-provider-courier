package courier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"go.uber.org/multierr"
	"golang.org/x/crypto/ssh"

	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/osx"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
	"github.com/seal-io/terraform-provider-courier/utils/testx"
	"github.com/seal-io/terraform-provider-courier/utils/version"
)

func TestProvider_metadata(t *testing.T) {
	var (
		ctx  = context.TODO()
		req  = provider.MetadataRequest{}
		resp = &provider.MetadataResponse{}
	)

	p := NewProvider()
	p.Metadata(ctx, req, resp)
	assert.Equal(t, resp.TypeName, ProviderType)
	assert.Equal(t, resp.Version, version.Version)
}

var testAccProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"courier": providerserver.NewProtocol6WithError(NewProvider()),
}

type multipass struct {
	hosts     int
	binPath   string
	configDir string
	priKeyBs  []byte
	pubKeyBs  []byte
	recreate  bool
}

func getMultipass(hosts int) (*multipass, error) {
	if hosts <= 0 {
		hosts = 1
	}

	binPath, err := exec.LookPath("multipass")
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	return &multipass{
		hosts:     hosts,
		binPath:   binPath,
		configDir: testx.AbsolutePath(".multipass"),
	}, nil
}

func (m *multipass) Start(t *testing.T, ctx context.Context) (err error) {
	err = m.configure(t)
	if err != nil {
		return err
	}

	for i := 0; i < m.hosts; i++ {
		n := m.name(i)

		if in, gerr := m.get(t, ctx, n); gerr == nil {
			if !m.recreate {
				if in.State == "Running" {
					continue
				}

				// Start.
				if multierr.AppendInto(&err, m.start(t, ctx, n)) {
					continue
				}
			}

			// Delete and recreate later.
			if multierr.AppendInto(&err, m.delete(t, ctx, n)) {
				continue
			}
		}

		err = multierr.Append(err, m.create(t, ctx, n))
	}

	return err
}

func (m *multipass) Stop(t *testing.T, ctx context.Context) (err error) {
	if strings.EqualFold(os.Getenv("TF_ACC_IGNORE_CLEAN"), "1") {
		return
	}

	for i := m.hosts - 1; i >= 0; i-- {
		n := m.name(i)

		err = multierr.Append(err, m.delete(t, ctx, n))
	}

	return err
}

func (m *multipass) GetEndpoints(
	t *testing.T,
	ctx context.Context,
) (priKey string, hosts []string, err error) {
	list, err := m.list(t, ctx)
	if err != nil {
		return "", nil, err
	}

	for i := range list {
		if len(list[i].IPv4) == 0 {
			continue
		}

		hosts = append(hosts, list[i].IPv4[0])
		if len(hosts) == m.hosts {
			break
		}
	}

	return string(m.priKeyBs), hosts, nil
}

func (m *multipass) configure(t *testing.T) (err error) {
	var (
		priKeyFile    = filepath.Join(m.configDir, "id_rsa")
		pubKeyFile    = filepath.Join(m.configDir, "id_rsa.pub")
		cloudInitFile = filepath.Join(m.configDir, "cloud-init.yaml")

		files = []string{
			priKeyFile,
			pubKeyFile,
			cloudInitFile,
		}
	)

	if osx.Exists(files...) {
		m.priKeyBs, err = os.ReadFile(priKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read private key file: %w", err)
		}

		m.pubKeyBs, err = os.ReadFile(pubKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read public key file: %w", err)
		}

		priKeyBlock, _ := pem.Decode(m.priKeyBs)
		priKey, err := x509.ParsePKCS1PrivateKey(priKeyBlock.Bytes)
		if err != nil {
			t.Logf("[MP ERROR] failed to parse private key: %v", err)
			goto generate
		}

		pubKey, _, _, _, err := ssh.ParseAuthorizedKey(m.pubKeyBs)
		if err != nil {
			t.Logf("[MP ERROR] failed to parse public key: %v", err)
			goto generate
		}

		pubKey_, ok := pubKey.(ssh.CryptoPublicKey)
		if !ok {
			t.Logf(
				"[MP ERROR] failed to convert public key to crypto public key",
			)
			goto generate
		}

		if !priKey.PublicKey.Equal(pubKey_.CryptoPublicKey()) {
			t.Logf(
				"[MP ERROR] public key does not match private key, regenerate ...",
			)
			goto generate
		}

		return nil
	}

generate:

	if err = os.MkdirAll(m.configDir, 0o700); err != nil {
		return err
	}

	defer func() {
		if err == nil {
			return
		}

		for i := range files {
			_ = os.Remove(files[i])
		}
	}()

	// Generate SSH keypair.
	t.Log("[MP INFO] generating SSH keypair ...")

	m.priKeyBs, m.pubKeyBs, err = func() ([]byte, []byte, error) {
		priKey, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, nil, err
		}

		pubKey, err := ssh.NewPublicKey(&priKey.PublicKey)
		if err != nil {
			return nil, nil, err
		}

		priKeyBs := pem.EncodeToMemory(&pem.Block{
			Type:    "RSA PRIVATE KEY",
			Headers: nil,
			Bytes:   x509.MarshalPKCS1PrivateKey(priKey),
		})
		if err = os.WriteFile(priKeyFile, priKeyBs, 0o600); err != nil {
			return nil, nil, fmt.Errorf(
				"failed to write private key file: %w",
				err,
			)
		}

		pubKeyBs := ssh.MarshalAuthorizedKey(pubKey)
		if err = os.WriteFile(pubKeyFile, pubKeyBs, 0o644); err != nil {
			return nil, nil, fmt.Errorf(
				"failed to write public key file: %w",
				err,
			)
		}

		return priKeyBs, pubKeyBs, nil
	}()
	if err != nil {
		return fmt.Errorf("failed to generate ssh keypair: %w", err)
	}

	// Generate cloud init configuration.
	t.Log("[MP INFO] generating cloud init configuration ...")

	cloudInitBs := []byte(fmt.Sprintf(`
ssh_pwauth: true

users:
  - default
  - name: ansible
    groups: staff,sudo,wheel
    lock_passwd: false
    passwd: $5$rounds=4096$shj749w9ri0MMGtL$WU4oIhMPFW4uk0M9jMVbTaJlQPFEcoADUDsp7QBIM55
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
  - name: root
    lock_passwd: false
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
    - %s
`, string(m.pubKeyBs)))
	if err = os.WriteFile(cloudInitFile, cloudInitBs, 0o644); err != nil {
		return fmt.Errorf("failed to generate cloud init: %w", err)
	}

	m.recreate = true

	return nil
}

func (m *multipass) create(
	t *testing.T,
	ctx context.Context,
	name string,
) error {
	return m.exec(t, ctx, io.Discard, "launch",
		"--cpus", "1",
		"--memory", "512M",
		"--disk", "5G",
		"--name", name,
		"--cloud-init", filepath.Join(m.configDir, "cloud-init.yaml"),
		"22.04",
	)
}

type multipassInstance struct {
	IPv4    []string `json:"ipv4"`
	Name    string   `json:"name"`
	Release string   `json:"release"`
	State   string   `json:"state"`
}

func (m *multipass) get(
	t *testing.T,
	ctx context.Context,
	name string,
) (multipassInstance, error) {
	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	// {
	//    "errors": [
	//    ],
	//    "info": {
	//        "courier-0": {
	//            "cpu_count": "1",
	//            "disks": {
	//                "sda1": {
	//                    "total": "5123149824",
	//                    "used": "1505986048"
	//                }
	//            },
	//            "image_hash": "bbf52c59f6a732087c373bcaeb582f19a62e4036bf7c84d2e7ffb7bf35fbcbea",
	//            "image_release": "22.04 LTS",
	//            "ipv4": [
	//                "192.168.64.9"
	//            ],
	//            "load": [
	//                0.01,
	//                0.08,
	//                0.04
	//            ],
	//            "memory": {
	//                "total": 482533376,
	//                "used": 122626048
	//            },
	//            "mounts": {
	//            },
	//            "release": "Ubuntu 22.04.3 LTS",
	//            "state": "Running"
	//        }
	//    }
	// }.
	err := m.exec(t, ctx, buf, "info",
		"--format", "json",
		name)
	if err != nil {
		return multipassInstance{}, err
	}

	var res struct {
		Info map[string]multipassInstance `json:"info"`
	}
	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return multipassInstance{}, fmt.Errorf(
			"failed to unmarshal get result: %w",
			err,
		)
	}

	if len(res.Info) == 0 {
		return multipassInstance{}, fmt.Errorf(
			"instance '%s' not found",
			name,
		)
	}

	return res.Info[name], nil
}

func (m *multipass) list(
	t *testing.T,
	ctx context.Context,
) ([]multipassInstance, error) {
	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	// {
	//    "list": [
	//        {
	//            "ipv4": [
	//                "192.168.64.9"
	//            ],
	//            "name": "courier-0",
	//            "release": "Ubuntu 22.04 LTS",
	//            "state": "Running"
	//        }
	//    ]
	// }.
	err := m.exec(t, ctx, buf, "list",
		"--format", "json")
	if err != nil {
		return nil, err
	}

	var res struct {
		List []multipassInstance `json:"list"`
	}
	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal list result: %w", err)
	}
	return res.List, nil
}

func (m *multipass) delete(
	t *testing.T,
	ctx context.Context,
	name string,
) error {
	return m.exec(t, ctx, io.Discard, "delete",
		"--purge", name)
}

func (m *multipass) start(
	t *testing.T,
	ctx context.Context,
	name string,
) error {
	return m.exec(t, ctx, io.Discard, "start",
		name)
}

func (m *multipass) exec(
	t *testing.T,
	ctx context.Context,
	wr io.Writer,
	args ...string,
) error {
	c := exec.CommandContext(ctx, m.binPath, args...)

	c.Stdout = io.MultiWriter(wr, stdoutWriter(t.Log))
	c.Stderr = stderrWriter(t.Log)

	err := c.Run()
	if err != nil {
		return fmt.Errorf(
			"failed to exec 'multipass %s': %w",
			strx.Join(" ", args...),
			err,
		)
	}

	return nil
}

func (m *multipass) name(i int) string {
	return "courier-" + strconv.Itoa(i)
}

type (
	stdoutWriter func(args ...any)
	stderrWriter func(args ...any)
)

func (w stdoutWriter) Write(p []byte) (int, error) {
	switch {
	default:
		w("[MP INFO] ", string(p))
	case len(p) != 0 && p[0] == '\b':
	}

	return len(p), nil
}

func (w stderrWriter) Write(p []byte) (int, error) {
	switch {
	default:
		w("[MP ERROR] ", string(p))
	case len(p) != 0 && p[0] == '\b':
	}

	return len(p), nil
}
