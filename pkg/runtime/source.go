package runtime

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/seal-io/terraform-provider-courier/utils/osx"
)

//go:embed source_builtin/*
var builtin embed.FS

func BuiltinSource() fs.FS {
	d, err := fs.Sub(builtin, "source_builtin")
	if err != nil {
		panic(fmt.Errorf("failed to get builtin source: %w", err))
	}

	return d
}

type (
	ExternalSourceOptions struct {
		Source   string
		Authn    ExternalSourceOptionAuthn
		Insecure bool
	}

	ExternalSourceOptionAuthn struct {
		Type   string
		User   string
		Secret string
	}
)

func ExternalSource(ctx context.Context, opts ExternalSourceOptions) (fs.FS, error) {
	srcURL, err := url.Parse(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse external source URL: %w", err)
	}

	var subpath string

	srcURL.Path, subpath, _ = strings.Cut(srcURL.Path, "//")

	cloneOpts := &git.CloneOptions{
		Depth:           1,
		Progress:        progress(func(p []byte) { tflog.Debug(ctx, string(p)) }),
		InsecureSkipTLS: opts.Insecure,
	}

	if q := srcURL.Query(); q != nil {
		ref := q.Get("ref")
		if ref != "" {
			cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(ref)
		}

		srcURL.RawQuery = ""
	}

	cloneOpts.URL = srcURL.String()

	switch au := opts.Authn; au.Type {
	case "basic":
		cloneOpts.Auth = &http.BasicAuth{
			Username: au.User,
			Password: au.Secret,
		}
	case "bearer":
		cloneOpts.Auth = &http.TokenAuth{
			Token: au.Secret,
		}
	}

	r, err := git.PlainCloneContext(ctx, osx.TempDir("courier-"), false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone git external source: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree from the git external source: %w", err)
	}

	wtDir := wt.Filesystem

	if subpath != "" {
		wtDir, err = wtDir.Chroot(subpath)
		if err != nil {
			return nil, fmt.Errorf("failed to chroot subpath of git external source: %w", err)
		}
	}

	return directory{wtDir: wtDir}, nil
}

type progress func(p []byte)

func (w progress) Write(p []byte) (n int, err error) {
	w(p)

	return len(p), nil
}

type directory struct {
	wtDir billy.Filesystem
}

func (d directory) Open(path string) (fs.File, error) {
	f, err := d.wtDir.Open(path)
	if err != nil {
		return nil, err
	}

	return file{File: f, wtDir: d.wtDir}, nil
}

type file struct {
	billy.File

	wtDir billy.Filesystem
}

func (f file) Stat() (fs.FileInfo, error) {
	return f.wtDir.Stat(f.Name())
}
