package target

import (
	"errors"

	"github.com/seal-io/terraform-provider-courier/pkg/target/ssh"
	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/pkg/target/winrm"
)

type (
	Host       = types.Host
	HostStatus = types.HostStatus

	HostOptions     = types.HostOptions
	HostOption      = types.HostOption
	HostOptionAuthn = types.HostOptionAuthn
)

var ErrUnknownHostAuthnType = errors.New("unknown host authn type")

func NewHost(opts HostOptions) (Host, error) {
	switch opts.Authn.Type {
	default:
		return nil, ErrUnknownHostAuthnType
	case "ssh":
		return ssh.New(opts)
	case "winrm":
		return winrm.New(opts)
	}
}
