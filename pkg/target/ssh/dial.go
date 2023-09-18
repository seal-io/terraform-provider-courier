package ssh

import (
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
)

func Dial(forward types.DialCloser, dialHost types.HostOption) (*ssh.Client, error) {
	ap, err := dialHost.ParseAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy address: %w", err)
	}

	var auths []ssh.AuthMethod

	if dialHost.Authn.Agent {
		agentConn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		if err != nil {
			return nil, fmt.Errorf("failed to connect SSH agent: %w", err)
		}

		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers))
	} else if dialHost.Authn.Secret != "" {
		pb, _ := pem.Decode([]byte(dialHost.Authn.Secret))

		if pb == nil {
			auths = append(auths, ssh.Password(dialHost.Authn.Secret))
		} else {
			if pb.Headers["Proc-Type"] == "4,ENCRYPTED" {
				return nil, errors.New("encrypted private key is not supported")
			}

			singer, err := ssh.ParsePrivateKey([]byte(dialHost.Authn.Secret))
			if err != nil {
				return nil, fmt.Errorf("faield to parse private key: %w", err)
			}

			auths = append(auths, ssh.PublicKeys(singer))
		}
	}

	cfg := &ssh.ClientConfig{
		User: dialHost.Authn.User,
		Auth: auths,
	}

	if dialHost.Insecure {
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey() //nolint:gosec
	}

	addr := ap.HostPort(22)

	if forward == nil {
		return ssh.Dial("tcp", addr, cfg)
	}

	prevConn, err := forward.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s via SSH: %w", addr, err)
	}

	conn, nextCh, nextReq, err := ssh.NewClientConn(prevConn, addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client connection: %w", err)
	}

	return ssh.NewClient(conn, nextCh, nextReq), nil
}
