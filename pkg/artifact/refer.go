package artifact

import (
	"errors"

	"github.com/seal-io/terraform-provider-courier/pkg/artifact/conimg"
	"github.com/seal-io/terraform-provider-courier/pkg/artifact/http"
	"github.com/seal-io/terraform-provider-courier/pkg/artifact/types"
)

type (
	Refer       = types.Refer
	ReferStatus = types.ReferStatus

	ReferOptions     = types.ReferOptions
	ReferOptionAuthn = types.ReferOptionAuthn
)

var ErrUnknownReferType = errors.New("unknown refer type")

func NewPackage(opts ReferOptions) (Refer, error) {
	switch opts.Type() {
	default:
		return nil, ErrUnknownReferType
	case types.ReferTypeContainerImage:
		return conimg.New(opts)
	case types.ReferTypeHTTP:
		return http.New(opts)
	}
}
