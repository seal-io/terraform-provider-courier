package http

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/seal-io/terraform-provider-courier/pkg/artifact/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/version"
)

type Package struct {
	url string
	rt  http.RoundTripper
}

func New(opts types.ReferOptions) (types.Refer, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if opts.Insecure {
		tr.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint: gosec
		}
	}

	var rt http.RoundTripper = tr

	rt = withUserAgent(rt, version.GetUserAgent())

	switch au := opts.Authn; au.Type {
	case "basic":
		rt = withBasicAuth(rt, au.User, au.Secret)
	case "bearer":
		rt = withBearerAuth(rt, au.Secret)
	}

	return &Package{
		url: opts.URI,
		rt:  rt,
	}, nil
}

func (p *Package) State(ctx context.Context) (types.ReferStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return types.ReferStatus{}, fmt.Errorf(
			"failed to create request: %w",
			err,
		)
	}

	cli := http.Client{Transport: p.rt}

	resp, err := cli.Do(req)
	if err != nil {
		return types.ReferStatus{}, fmt.Errorf(
			"failed to do request: %w",
			err,
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return types.ReferStatus{}, fmt.Errorf(
			"unexpected status code: %d",
			resp.StatusCode,
		)
	}

	hash := sha256.New()

	buf := bytespool.GetBytes()
	defer func() { bytespool.Put(buf) }()

	length, err := io.CopyBuffer(hash, resp.Body, buf)
	if err != nil {
		return types.ReferStatus{}, fmt.Errorf(
			"failed to hash response: %w",
			err,
		)
	}

	digest := hex.EncodeToString(hash.Sum(nil))

	return types.ReferStatus{
		Accessible: true,
		Digest:     "sha256:" + digest,
		Type:       resp.Header.Get("Content-Type"),
		Length:     length,
	}, nil
}
