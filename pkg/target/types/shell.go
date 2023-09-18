package types

import (
	"context"
	"io"
)

type (
	Sheller interface {
		// Shell returns a terminal to execute multiple commands on the host.
		Shell(ctx context.Context, cmdArgs ...string) (Terminal, error)
	}

	Terminal interface {
		io.ReadWriteCloser

		// Execute executes the given command on the host.
		Execute(cmd string, args ...string) error
		// ExecuteWithOutput executes the given command on the host and returns the output.
		ExecuteWithOutput(cmd string, args ...string) ([]byte, error)
	}
)
