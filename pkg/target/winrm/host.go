package winrm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/masterzen/winrm"

	"github.com/seal-io/terraform-provider-courier/pkg/target/codec"
	"github.com/seal-io/terraform-provider-courier/pkg/target/proxy"
	"github.com/seal-io/terraform-provider-courier/pkg/target/ssh"
	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

type Host struct {
	client *winrm.Client
}

func New(opts types.HostOptions) (types.Host, error) {
	if opts.Authn.Type != "winrm" {
		return nil, errors.New("invalid type")
	}

	if opts.Address == "" {
		return nil, errors.New("no address specified")
	}

	proxies, err := proxyWith(types.DialClosers{}, opts.Proxies)
	if err != nil {
		return nil, fmt.Errorf("failed to dail %s via proxies: %w", opts.Address, err)
	}

	c, err := Dial(proxies, opts.HostOption)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", opts.Address, err)
	}

	return &Host{
		client: c,
	}, nil
}

func proxyWith(pds types.DialClosers, dhs []types.HostOption) (d types.DialCloser, err error) {
	if len(dhs) == 0 {
		return pds, nil
	}

	da, dhs := dhs[0], dhs[1:]

	switch da.Authn.Type {
	default:
		return nil, errors.New("unknown host type")
	case "ssh":
		d, err = ssh.Dial(pds, da)
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
	return nil
}

func (h *Host) State(ctx context.Context) (types.HostStatus, error) {
	t, err := h.Shell(ctx)
	if err != nil {
		return types.HostStatus{}, err
	}
	defer func() { _ = t.Close() }()

	// Refer to https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-processor.
	archBs, err := t.ExecuteWithOutput(
		winrm.Powershell(
			"Get-WmiObject Win32_Processor -Property Architecture | Select-Object -ExpandProperty Architecture",
		),
	)
	if err != nil {
		return types.HostStatus{}, fmt.Errorf("failed to get arch: %w", err)
	}
	arch := strings.ToLower(strx.FromBytes(&archBs))

	switch arch {
	case "0": // X86.
		arch = "386"
	case "1": // MIPS.
		arch = "mips"
	case "2": // Alpha.
		arch = "alpha"
	case "3": // PowerPC.
		arch = "ppc"
	case "5": // ARM.
		arch = "arm"
	case "6": // Ia64.
		arch = "ia64"
	case "9": // X64.
		arch = "amd64"
	case "12": // ARM64.
		arch = "arm64"
	default:
		arch = ""
	}

	// Refer to https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-operatingsystem.
	versionBs, err := t.ExecuteWithOutput(
		winrm.Powershell(
			"Get-WmiObject Win32_OperatingSystem -Property Version | Select-Object -ExpandProperty Version",
		),
	)
	if err != nil {
		return types.HostStatus{}, fmt.Errorf("failed to get kernel version: %w", err)
	}
	version := strings.ToLower(strx.FromBytes(&versionBs))

	return types.HostStatus{
		Accessible: true,
		OS:         "windows",
		Arch:       arch,
		Version:    version,
	}, nil
}

func (h *Host) Execute(ctx context.Context, cmd string, args ...string) error {
	return h.execute(ctx, io.Discard, cmd, args)
}

func (h *Host) ExecuteWithOutput(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	err := h.execute(ctx, buf, cmd, args)
	return buf.Bytes(), err
}

func (h *Host) execute(ctx context.Context, out io.Writer, cmd string, args []string) error {
	if cmd == "" {
		return errors.New("blank command")
	}

	command := codec.EncodeShellInput("windows", cmd, args, "")

	_, err := h.client.RunWithContext(ctx, command, out, out)
	return err
}
