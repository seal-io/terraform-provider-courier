package ssh

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/seal-io/terraform-provider-courier/pkg/target/codec"
	"github.com/seal-io/terraform-provider-courier/pkg/target/proxy"
	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/iox"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

type Host struct {
	client   *ssh.Client
	platform string
}

func New(opts types.HostOptions) (types.Host, error) {
	if opts.Authn.Type != "ssh" {
		return nil, errors.New("invalid type")
	}

	if opts.Address == "" {
		return nil, errors.New("no address specified")
	}

	proxies, err := proxyWith(types.DialClosers{}, opts.Proxies)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to dail %s via proxies: %w",
			opts.Address,
			err,
		)
	}

	c, err := Dial(proxies, opts.HostOption)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", opts.Address, err)
	}

	return &Host{
		client:   c,
		platform: "linux",
	}, nil
}

func proxyWith(
	pds types.DialClosers,
	dhs []types.HostOption,
) (d types.DialCloser, err error) {
	if len(dhs) == 0 {
		return pds, nil
	}

	da, dhs := dhs[0], dhs[1:]

	switch da.Authn.Type {
	default:
		return nil, errors.New("unknown host type")
	case "ssh":
		d, err = Dial(pds, da)
	case "proxy":
		d, err = proxy.Dial(pds, da)
	}

	if err != nil {
		_ = pds.Close()
		return nil, err
	}

	if pds != nil {
		pds = pds[:len(pds):len(pds)]
	}

	return proxyWith(append(pds, d), dhs)
}

func (h *Host) Close() error {
	return h.client.Close()
}

func (h *Host) State(ctx context.Context) (types.HostStatus, error) {
	t, err := h.Shell(ctx)
	if err != nil {
		return types.HostStatus{}, err
	}
	defer func() { _ = t.Close() }()

	osBs, err := t.ExecuteWithOutput("uname", "-s")
	if err != nil {
		return types.HostStatus{}, fmt.Errorf("failed to get os: %w", err)
	}
	os := strings.ToLower(strx.FromBytes(&osBs))

	// Refer to https://stackoverflow.com/questions/45125516/possible-values-for-uname-m.
	archBs, err := t.ExecuteWithOutput("uname", "-m")
	if err != nil {
		return types.HostStatus{}, fmt.Errorf(
			"failed to get arch: %w",
			err,
		)
	}
	arch := strings.ToLower(strx.FromBytes(&archBs))

	switch {
	case arch == "x86_64":
		arch = "amd64"
	case strings.HasSuffix(arch, "aarch64") || strings.HasSuffix(arch, "armv8"):
		arch = "arm64"
	case strings.HasPrefix(arch, "riscv"):
		arch = "riscv64"
	case arch == "i386", arch == "i686", arch == "x86":
		arch = "386"
	case strings.HasPrefix(arch, "arm"):
		arch = "arm"
	}

	versionBs, err := t.ExecuteWithOutput("uname", "-r")
	if err != nil {
		return types.HostStatus{}, fmt.Errorf(
			"failed to get kernel version: %w",
			err,
		)
	}
	version := strings.ToLower(strx.FromBytes(&versionBs))

	return types.HostStatus{
		Accessible: true,
		OS:         os,
		Arch:       arch,
		Version:    version,
	}, nil
}

func (h *Host) Execute(
	ctx context.Context,
	cmd string,
	args ...string,
) error {
	if cmd == "" {
		return errors.New("blank command")
	}

	s, err := h.getSessionWithContext(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = s.Close() }()

	command := codec.EncodeExecInput(h.platform, cmd, args)

	return s.Run(command)
}

func (h *Host) ExecuteWithOutput(
	ctx context.Context,
	cmd string,
	args ...string,
) ([]byte, error) {
	if cmd == "" {
		return nil, errors.New("blank command")
	}

	s, err := h.getSessionWithContext(ctx)
	if err != nil {
		return nil, err
	}

	defer func() { _ = s.Close() }()

	command := codec.EncodeExecInput(h.platform, cmd, args)

	return s.CombinedOutput(command)
}

type session struct {
	*ssh.Session
	context.Context

	Cancel context.CancelFunc
}

func (h *Host) getSessionWithContext(
	ctx context.Context,
) (*session, error) {
	s, err := h.client.NewSession()
	if err != nil {
		return nil, err
	}

	_, err = s.SendRequest("keepalive", true, nil)
	if err != nil {
		return nil, fmt.Errorf("disconnected: %w", err)
	}

	sCtx, sCtxCancel := context.WithCancel(ctx)
	cs := &session{
		Session: s,
		Context: sCtx,
		Cancel:  sCtxCancel,
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = s.Signal(ssh.SIGKILL)
			cs.Cancel()
		case <-cs.Done():
		}
	}()

	return cs, nil
}

func (s *session) Close() error {
	defer s.Cancel()

	return s.Session.Close()
}

func (s *session) Output(cmd string) ([]byte, error) {
	if s.Stdout != nil {
		return nil, errors.New("ssh: Stdout already set")
	}

	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	s.Stdout = buf

	err := s.Run(cmd)
	return buf.Bytes(), err
}

func (s *session) CombinedOutput(cmd string) ([]byte, error) {
	if s.Stdout != nil {
		return nil, errors.New("ssh: Stdout already set")
	}

	if s.Stderr != nil {
		return nil, errors.New("ssh: Stderr already set")
	}

	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	wr := iox.SingleWriter(buf)
	s.Stdout = wr
	s.Stderr = wr

	err := s.Run(cmd)
	return buf.Bytes(), err
}
