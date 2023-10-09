package ssh

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"github.com/pkg/sftp"

	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
)

func (h *Host) UploadFile(
	ctx context.Context,
	from types.FileReader,
	to string,
) (err error) {
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

	buf := bytespool.GetBytes()
	defer func() { bytespool.Put(buf) }()

	_, err = io.CopyBuffer(wr, from, buf)
	return err
}

func (h *Host) UploadDirectory(
	ctx context.Context,
	from types.DirectoryReader,
	to string,
) error {
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

	return fs.WalkDir(
		from,
		".",
		func(p string, d fs.DirEntry, ierr error) (err error) {
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

			buf := bytespool.GetBytes()
			defer func() { bytespool.Put(buf) }()

			_, err = io.CopyBuffer(wr, rd, buf)
			return err
		},
	)
}

type file struct {
	*sftp.File

	ft *fileTransport
}

func (f file) Close() error {
	defer func() { _ = f.ft.Close() }()

	return f.File.Close()
}

func (h *Host) DownloadFile(
	ctx context.Context,
	from string,
) (types.FileReadCloser, error) {
	if from == "" {
		return nil, errors.New("blank remote file path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return nil, err
	}

	f, err := func() (*sftp.File, error) {
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

	return file{
		File: f,
		ft:   c,
	}, nil
}

type directory struct {
	ft *fileTransport
}

func (d directory) Close() error {
	return d.ft.Close()
}

func (d directory) Open(name string) (fs.File, error) {
	return d.ft.Open(name)
}

func (d directory) ReadDir(name string) ([]fs.DirEntry, error) {
	ds, err := d.ft.ReadDir(name)
	if err != nil {
		return nil, err
	}

	r := make([]fs.DirEntry, len(ds))
	for i := range ds {
		r[i] = fs.FileInfoToDirEntry(ds[i])
	}
	return r, nil
}

func (h *Host) DownloadDirectory(
	ctx context.Context,
	from string,
) (types.DirectoryReadCloser, error) {
	if from == "" {
		return nil, errors.New("blank remote directory path")
	}

	c, err := h.getFileTransportWithContext(ctx)
	if err != nil {
		return nil, err
	}

	err = func() error {
		i, err := c.Lstat(from)
		if err != nil {
			return err
		}

		if !i.IsDir() {
			return errors.New("remote path is not a directory")
		}

		return nil
	}()
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	return directory{
		ft: c,
	}, nil
}

type fileTransport struct {
	*sftp.Client

	session *session
}

func (h *Host) getFileTransportWithContext(
	ctx context.Context,
) (*fileTransport, error) {
	s, err := h.getSessionWithContext(ctx)
	if err != nil {
		return nil, err
	}

	c, err := func() (*sftp.Client, error) {
		if err = s.RequestSubsystem("sftp"); err != nil {
			return nil, err
		}

		rd, err := s.StdoutPipe()
		if err != nil {
			return nil, err
		}

		wr, err := s.StdinPipe()
		if err != nil {
			return nil, err
		}

		return sftp.NewClientPipe(rd, wr)
	}()
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	return &fileTransport{
		Client:  c,
		session: s,
	}, nil
}

func (ft *fileTransport) Close() error {
	defer func() { _ = ft.session.Close() }()

	return ft.Client.Close()
}

func (ft *fileTransport) Open(name string) (*sftp.File, error) {
	return ft.Client.Open(name)
}
