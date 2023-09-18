package winrm

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
)

func (h *Host) UploadFile(ctx context.Context, from types.FileReader, to string) (err error) {
	if from == nil {
		return errors.New("nil local file reader")
	}

	if to == "" {
		return errors.New("blank remote file path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = c.Close() }()

	wr, err := c.Create(to)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := wr.Close(); err == nil {
			err = cerr
		}
	}()

	// NB(thxCode): Inspired by
	// https://github.com/packer-community/winrmcp/blob/6e900dd2c68f81845f61265562b6299a806162e0/winrmcp/cp.go.
	bs := ((8000 - len(to)) / 4) * 3

	buf := bytespool.GetBytes(bs)
	defer func() { bytespool.Put(buf) }()

	_, err = io.CopyBuffer(wr, from, buf)
	return err
}

func (h *Host) UploadDirectory(ctx context.Context, from types.DirectoryReader, to string) error {
	if from == nil {
		return errors.New("nil local directory reader")
	}

	if to == "" {
		return errors.New("blank remote directory path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = c.Close() }()

	err = c.MkdirAll(to)
	if err != nil {
		return err
	}

	return fs.WalkDir(from, ".", func(p string, d fs.DirEntry, ierr error) (err error) {
		if ierr != nil {
			return ierr
		}

		if d.IsDir() {
			return c.MkdirAll(to + "/" + p)
		}

		rd, err := from.Open(p)
		if err != nil {
			return err
		}

		defer func() { _ = rd.Close() }()

		wr, err := c.Create(to + "/" + p)
		if err != nil {
			return err
		}

		defer func() {
			if cerr := wr.Close(); err == nil {
				err = cerr
			}
		}()

		// NB(thxCode): Inspired by
		// https://github.com/packer-community/winrmcp/blob/6e900dd2c68f81845f61265562b6299a806162e0/winrmcp/cp.go.
		bs := ((8000 - len(to)) / 4) * 3

		buf := bytespool.GetBytes(bs)
		defer func() { bytespool.Put(buf) }()

		_, err = io.CopyBuffer(wr, rd, buf)
		return err
	})
}

func (h *Host) DownloadFile(ctx context.Context, from string) (types.FileReadCloser, error) {
	if from == "" {
		return nil, errors.New("blank remote file path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return nil, err
	}

	f, err := func() (fs.File, error) {
		i, err := c.Lstat(from)
		if err != nil {
			return nil, err
		}

		if i.IsDir() {
			return nil, errors.New("remote path is not a file")
		}

		return c.Open(from)
	}()
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	return f, nil
}

func (h *Host) DownloadDirectory(ctx context.Context, from string) (types.DirectoryReadCloser, error) {
	if from == "" {
		return nil, errors.New("blank remote directory path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (h *Host) getFileTransportWithContext(ctx context.Context) (*fileTransport, error) {
	s, err := h.client.CreateShell()
	if err != nil {
		return nil, err
	}

	return &fileTransport{
		ctx:   ctx,
		shell: s,
	}, nil
}
