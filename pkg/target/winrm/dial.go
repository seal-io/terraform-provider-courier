package winrm

import (
	"fmt"
	"time"

	"github.com/masterzen/winrm"

	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
)

func Dial(forward types.DialCloser, dialHost types.HostOption) (*winrm.Client, error) {
	ap, err := dialHost.ParseAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy address: %w", err)
	}

	if ap.Port == 0 {
		ap.Port = 5985
	}

	ep := winrm.NewEndpoint(
		ap.Host,
		ap.Port,
		ap.Scheme == "https",
		dialHost.Insecure,
		nil,
		nil,
		nil,
		15*time.Second,
	)
	ps := winrm.NewParameters("PT60S", "en-US", 153600)

	if forward != nil {
		ps.Dial = forward.Dial
	}

	if dialHost.Authn.Agent {
		ps.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientNTLM{}
		}
	}

	cli, err := winrm.NewClientWithParameters(ep, dialHost.Authn.User, dialHost.Authn.Secret, ps)
	if err != nil {
		return nil, fmt.Errorf("failed to create WinRM client connection: %w", err)
	}

	return cli, nil
}
