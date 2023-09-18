package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/sync/errgroup"

	"github.com/seal-io/terraform-provider-courier/pkg/target/codec"
	"github.com/seal-io/terraform-provider-courier/pkg/target/types"
	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

type Terminal struct {
	io.WriteCloser
	io.Reader

	platform string
	session  *session
	echo     string
}

func (h *Host) Shell(ctx context.Context, cmdArgs ...string) (types.Terminal, error) {
	echo := fmt.Sprintf(`#%s#`, strx.Hex(8))

	s, err := h.getSessionWithContext(ctx)
	if err != nil {
		return nil, err
	}

	i, o, e, err := func() (stdin io.WriteCloser, stdout, stderr io.Reader, err error) {
		stdin, err = s.StdinPipe()
		if err != nil {
			return
		}

		stdout, err = s.StdoutPipe()
		if err != nil {
			return
		}

		stderr, err = s.StderrPipe()
		if err != nil {
			return
		}

		var (
			cmd  string
			args []string
		)

		if len(cmdArgs) > 0 {
			cmd = cmdArgs[0]
			args = cmdArgs[1:]
		} else {
			cmd = "/bin/sh"
		}

		err = s.Start(codec.EncodeExecInput(h.platform, cmd, args))
		return
	}()
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	return &Terminal{
		WriteCloser: i,
		Reader:      io.MultiReader(o, e),
		platform:    h.platform,
		session:     s,
		echo:        echo,
	}, nil
}

func (t *Terminal) Close() error {
	defer func() { _ = t.session.Close() }()

	_ = t.WriteCloser.Close()

	return t.session.Wait()
}

func (t *Terminal) Execute(cmd string, args ...string) error {
	return t.doExecute(io.Discard, cmd, args)
}

func (t *Terminal) ExecuteWithOutput(cmd string, args ...string) ([]byte, error) {
	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	err := t.doExecute(buf, cmd, args)
	return buf.Bytes(), err
}

func (t *Terminal) doExecute(wr io.Writer, cmd string, args []string) error {
	if cmd == "" {
		return errors.New("blank command")
	}

	var g errgroup.Group

	g.Go(func() error {
		command := codec.EncodeShellInput(t.platform, cmd, args, t.echo)
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
