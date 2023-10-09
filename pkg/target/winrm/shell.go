package winrm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/masterzen/winrm"
	"golang.org/x/sync/errgroup"

	"github.com/seal-io/terraform-provider-courier/pkg/target/codec"
	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

type Terminal struct {
	io.WriteCloser
	io.Reader

	shell   *winrm.Shell
	command *winrm.Command
	echo    string
}

func (h *Host) Shell(
	ctx context.Context,
	cmdArgs ...string,
) (types.Terminal, error) {
	echo := fmt.Sprintf(`#%s#`, strx.Hex(8))

	s, err := h.client.CreateShell()
	if err != nil {
		return nil, err
	}

	var (
		cmd  string
		args []string
	)

	if len(cmdArgs) > 0 {
		cmd = cmdArgs[0]
		args = cmdArgs[1:]
	} else {
		cmd = "powershell.exe"
	}

	c, err := s.ExecuteWithContext(ctx, cmd, args...)
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	return &Terminal{
		WriteCloser: c.Stdin,
		Reader:      io.MultiReader(c.Stdout, c.Stderr),

		shell:   s,
		command: c,
		echo:    echo,
	}, nil
}

func (t *Terminal) Close() error {
	defer func() { _ = t.shell.Close() }()

	return t.command.Close()
}

func (t *Terminal) Execute(cmd string, args ...string) error {
	return t.execute(io.Discard, cmd, args)
}

func (t *Terminal) ExecuteWithOutput(
	cmd string,
	args ...string,
) ([]byte, error) {
	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	err := t.execute(buf, cmd, args)
	return buf.Bytes(), err
}

func (t *Terminal) execute(wr io.Writer, cmd string, args []string) error {
	if cmd == "" {
		return errors.New("blank command")
	}

	var g errgroup.Group

	g.Go(func() error {
		command := codec.EncodeShellInput("windows", cmd, args, t.echo)
		_, err := t.Write(strx.ToBytes(&command))
		return err
	})

	g.Go(func() error {
		s := bufio.NewScanner(t)
		for s.Scan() {
			output := s.Text()

			found, err := codec.DecodeShellOutput(&output, t.echo)
			if found {
				return err
			}

			_, err = wr.Write(strx.ToBytes(&output))
			if err != nil {
				return err
			}
		}

		return s.Err()
	})

	return g.Wait()
}
