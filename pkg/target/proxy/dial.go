package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"

	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
)

func Dial(
	forward types.DialCloser,
	dialHost types.HostOption,
) (types.DialCloser, error) {
	if forward == nil {
		forward = types.NopDialCloser(proxy.Direct)
	}

	ap, err := dialHost.ParseAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy address: %w", err)
	}

	au := &proxy.Auth{
		User:     dialHost.Authn.User,
		Password: dialHost.Authn.Secret,
	}

	switch s := ap.Scheme; s {
	default:
		return nil, fmt.Errorf("unknown proxy scheme: %s", ap.Scheme)
	case "http", "https":
		addr := ap.HostPortFunc(func(p types.HostAddressParsed) int {
			if p.Scheme == "https" {
				return 443
			}
			return 80
		})

		// NB(thxCode): Inspired by github.com/hashicorp/terraform/internal/communicator/ssh/http_proxy.go.
		d := types.DialCloserFunc(
			func(network, address string) (n net.Conn, err error) {
				n, err = forward.Dial(network, addr)
				if err != nil {
					return nil, err
				}

				defer func() {
					if err != nil {
						_ = n.Close()
					}
				}()

				err = n.SetDeadline(time.Now().Add(15 * time.Second))
				if err != nil {
					return nil, err
				}

				req, err := http.NewRequest(
					http.MethodConnect,
					address,
					nil,
				)
				if err != nil {
					return nil, err
				}

				if au != nil {
					req.SetBasicAuth(au.User, au.Password)
					req.Header.Add(
						"Proxy-Authorization",
						req.Header.Get("Authorization"),
					)
				}

				err = req.Write(n)
				if err != nil {
					return nil, err
				}

				resp, err := http.ReadResponse(bufio.NewReader(n), req)
				if err != nil {
					return nil, err
				}

				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					return nil, fmt.Errorf(
						"connection error: status code: %d",
						resp.StatusCode,
					)
				}

				return n, nil
			},
		)

		return d, nil
	case "socks5", "socks5h":
		addr := ap.HostPort(1080)

		d, err := proxy.SOCKS5("tcp", addr, au, forward)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create Socks5 proxy: %w",
				err,
			)
		}

		return types.NopDialCloser(d), nil
	}
}
