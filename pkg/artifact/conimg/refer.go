package conimg

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/seal-io/terraform-provider-courier/pkg/artifact/types"
	"github.com/seal-io/terraform-provider-courier/utils/version"
)

type Refer struct {
	ref  name.Reference
	opts []remote.Option
}

func New(opts types.ReferOptions) (types.Refer, error) {
	nameRefOpts := []name.Option{
		name.WithDefaultRegistry(name.DefaultRegistry),
		name.WithDefaultTag(name.DefaultTag),
		name.WeakValidation,
	}

	if opts.Insecure {
		nameRefOpts = append(nameRefOpts, name.Insecure)
	}

	nameRef, err := name.ParseReference(opts.URI, nameRefOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refer: %w", err)
	}

	remoteOpts := []remote.Option{
		remote.WithUserAgent(version.GetUserAgent()),
	}

	switch au := opts.Authn; au.Type {
	case "basic":
		remoteOpts = append(remoteOpts,
			remote.WithAuth(&authn.Basic{
				Username: au.User,
				Password: au.Secret,
			}))
	case "bearer":
		remoteOpts = append(remoteOpts,
			remote.WithAuth(&authn.Bearer{
				Token: au.Secret,
			}))
	}

	return &Refer{
		ref:  nameRef,
		opts: remoteOpts[:len(remoteOpts):len(remoteOpts)],
	}, nil
}

func (p *Refer) State(ctx context.Context) (types.ReferStatus, error) {
	d, err := remote.Head(p.ref, append(p.opts, remote.WithContext(ctx))...)
	if err != nil {
		return types.ReferStatus{}, fmt.Errorf("failed to head: %w", err)
	}

	return types.ReferStatus{
		Accessible: true,
		Digest:     d.Digest.String(),
		Type:       string(d.MediaType),
		Length:     d.Size,
	}, nil
}
