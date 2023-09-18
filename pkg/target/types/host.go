package types

import (
	"context"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type (
	Host interface {
		io.Closer
		Sheller
		Transmitter

		// State returns the status of the host.
		State(ctx context.Context) (HostStatus, error)
		// Execute executes the given command on the host.
		Execute(ctx context.Context, cmd string, args ...string) error
		// ExecuteWithOutput executes the given command on the host and returns the output.
		ExecuteWithOutput(ctx context.Context, cmd string, args ...string) ([]byte, error)
	}

	HostStatus struct {
		Accessible bool
		OS         string
		Arch       string
		Version    string
	}
)

type (
	HostOptions struct {
		HostOption

		Proxies []HostOption
	}

	HostOption struct {
		Address  string
		Authn    HostOptionAuthn
		Insecure bool
	}

	HostOptionAuthn struct {
		Type   string
		User   string
		Secret string
		Agent  bool
	}
)

type HostAddressParsed struct {
	Scheme string
	Host   string
	Port   int
}

func (o HostOption) ParseAddress() (parsed HostAddressParsed, err error) {
	r := o.Address

	if strings.Contains(r, "://") {
		var u *url.URL

		u, err = url.Parse(r)
		if err != nil {
			return
		}

		parsed.Scheme = u.Scheme
		parsed.Host = u.Host
		parsed.Port, err = strconv.Atoi(u.Port())

		return parsed, err
	}

	parsed.Host = r

	if h, p, _ := net.SplitHostPort(parsed.Host); h != "" {
		parsed.Host = h
		parsed.Port, err = strconv.Atoi(p)
	}

	return parsed, err
}

func (p HostAddressParsed) HostPort(defaultPort int) string {
	if p.Port <= 0 {
		p.Port = defaultPort
	}

	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

func (p HostAddressParsed) HostPortFunc(defaultPortFunc func(HostAddressParsed) int) string {
	if p.Port <= 0 && defaultPortFunc != nil {
		p.Port = defaultPortFunc(p)
	}

	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}
