package types

import (
	"context"
	"io"
	"io/fs"
)

type (
	FileReader          = io.Reader
	FileReadCloser      = io.ReadCloser
	DirectoryReader     = fs.FS
	DirectoryReadCloser = interface {
		fs.FS
		io.Closer
	}

	Transmitter interface {
		// UploadFile uploads the given file to the host.
		UploadFile(ctx context.Context, from FileReader, to string) error
		// UploadDirectory uploads the given directory to the host.
		UploadDirectory(ctx context.Context, from DirectoryReader, to string) error
		// DownloadFile downloads the given file from the host.
		DownloadFile(ctx context.Context, from string) (FileReadCloser, error)
		// DownloadDirectory downloads the given directory from the host.
		DownloadDirectory(ctx context.Context, from string) (DirectoryReadCloser, error)
	}
)
