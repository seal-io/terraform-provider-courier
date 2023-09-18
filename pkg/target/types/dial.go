package types

import (
	"io"
	"net"

	"go.uber.org/multierr"
	"golang.org/x/net/proxy"
)

type (
	DialCloser interface {
		io.Closer

		Dial(network, address string) (net.Conn, error)
	}

	DialClosers []DialCloser
)

func (p DialClosers) Close() (err error) {
	for i := range p {
		err = multierr.Append(err, p[i].Close())
	}

	return
}

func (p DialClosers) Dial(network, address string) (net.Conn, error) {
	if len(p) == 0 {
		return net.Dial(network, address)
	}

	return p[len(p)-1].Dial(network, address)
}

type nopCloser struct {
	proxy.Dialer
}

func NopDialCloser(d proxy.Dialer) DialCloser {
	return nopCloser{d}
}

func (nopCloser) Close() error {
	return nil
}

func (p nopCloser) Dial(network, addr string) (net.Conn, error) {
	return p.Dialer.Dial(network, addr)
}

type DialCloserFunc func(network, address string) (net.Conn, error)

func (DialCloserFunc) Close() error {
	return nil
}

func (f DialCloserFunc) Dial(network, addr string) (net.Conn, error) {
	return f(network, addr)
}
