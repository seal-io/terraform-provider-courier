package types

import (
	"context"
	"net/url"
	"strings"

	conregname "github.com/google/go-containerregistry/pkg/name"
)

type (
	Refer interface {
		// State returns the status of the reference.
		State(ctx context.Context) (ReferStatus, error)
	}

	ReferStatus struct {
		Accessible bool
		Digest     string
		Type       string
		Length     int64
	}
)

type (
	ReferOptions struct {
		URI      string
		Authn    ReferOptionAuthn
		Insecure bool
	}

	ReferOptionAuthn struct {
		Type   string
		User   string
		Secret string
	}
)

const (
	ReferTypeHTTP           = "http"
	ReferTypeContainerImage = "container_image"
)

func (o ReferOptions) Type() string {
	r := o.URI

	if strings.Contains(r, "://") {
		u, err := url.Parse(r)
		if err != nil {
			return ""
		}

		switch u.Scheme {
		default:
			return ""
		case "http", "https":
			return ReferTypeHTTP
		}
	}

	_, err := conregname.ParseReference(r)
	if err == nil {
		return ReferTypeContainerImage
	}

	return ""
}
