package http

import (
	"net/http"
)

var (
	_ http.RoundTripper = (*basic)(nil)
	_ http.RoundTripper = (*bearer)(nil)
	_ http.RoundTripper = (*userAgent)(nil)
)

type basic struct {
	http.RoundTripper

	username string
	password string
}

func withBasicAuth(
	in http.RoundTripper,
	username, password string,
) *basic {
	return &basic{
		RoundTripper: in,
		username:     username,
		password:     password,
	}
}

func (t *basic) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.SetBasicAuth(t.username, t.password)

	return t.RoundTripper.RoundTrip(r2)
}

type bearer struct {
	http.RoundTripper

	token string
}

func withBearerAuth(in http.RoundTripper, token string) *bearer {
	return &bearer{
		RoundTripper: in,
		token:        token,
	}
}

func (t *bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+t.token)

	return t.RoundTripper.RoundTrip(r2)
}

type userAgent struct {
	http.RoundTripper

	ua string
}

func withUserAgent(in http.RoundTripper, ua string) *userAgent {
	return &userAgent{
		RoundTripper: in,
		ua:           ua,
	}
}

func (t *userAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("User-Agent", t.ua)

	return t.RoundTripper.RoundTrip(r2)
}
