package winrm

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"

	"github.com/seal-io/terraform-provider-courier/utils/bytespool"
)

type fileTransport struct {
	ctx   context.Context
	shell *winrm.Shell
}

func (ft *fileTransport) Close() error {
	return ft.shell.Close()
}

func (ft *fileTransport) Create(path string) (*writableFile, error) {
	if path == "" {
		return nil, errors.New("blank path")
	}

	path = toWindowsPath(path)
	command := winrm.Powershell(fmt.Sprintf("New-Item -Force -ItemType File -Path %s", path))

	err := execute(ft.ctx, ft.shell, io.Discard, command)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %q: %w", path, err)
	}

	return &writableFile{
		ctx:   ft.ctx,
		shell: ft.shell,
		path:  path,
	}, nil
}

func (ft *fileTransport) Lstat(path string) (fs.FileInfo, error) {
	if path == "" {
		return nil, errors.New("blank path")
	}

	path = toWindowsPath(path)
	f := readonlyFile{
		ctx:   ft.ctx,
		shell: ft.shell,
		path:  path,
	}

	return f.Stat()
}

func (ft *fileTransport) Open(path string) (fs.File, error) {
	if path == "" {
		return nil, errors.New("blank path")
	}

	path = toWindowsPath(path)
	command := winrm.Powershell(fmt.Sprintf("Get-Item -Force -Path %s", path))

	err := execute(ft.ctx, ft.shell, io.Discard, command)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", path, err)
	}

	return &readonlyFile{
		ctx:   ft.ctx,
		shell: ft.shell,
		path:  path,
	}, nil
}

func (ft *fileTransport) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == "" {
		return nil, errors.New("blank path")
	}

	// TODO(thxCode): implement this.
	return nil, nil
}

func (ft *fileTransport) MkdirAll(path string) error {
	if path == "" {
		return errors.New("blank path")
	}

	path = toWindowsPath(path)
	command := winrm.Powershell(fmt.Sprintf("New-Item -Force -ItemType Directory -Path %s", path))

	err := execute(ft.ctx, ft.shell, io.Discard, command)
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %w", path, err)
	}

	return nil
}

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (fi fileInfo) Name() string {
	return fi.name
}

func (fi fileInfo) Size() int64 {
	return fi.size
}

func (fi fileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi fileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi fileInfo) IsDir() bool {
	return fi.mode.IsDir()
}

func (fi fileInfo) Sys() any {
	return nil
}

type writableFile struct {
	ctx   context.Context
	shell *winrm.Shell
	path  string
	err   error
}

func (f *writableFile) Close() error {
	if f.err != nil || f.ctx.Err() != nil {
		return f.shell.Close()
	}

	// NB(thxCode): Restore the file's content, inspired by
	// https://github.com/packer-community/winrmcp/blob/6e900dd2c68f81845f61265562b6299a806162e0/winrmcp/cp.go.
	command := winrm.Powershell(fmt.Sprintf(`
		$path = "%s"
		if (Test-Path ${path} -Type Leaf) {
			$rd = [System.IO.File]::OpenText(${path})
			$wr = [System.IO.File]::OpenWrite(${path}.tmp)
			try {
				for(;;) {
					$bs64 = $rd.ReadLine()
					if (${bs64} -eq $null) { break }
					$bs = [System.Convert]::FromBase64String(${bs64})
					$wr.Write(${bs}, 0, ${bs}.Length)
				}
				Move-Item -Path ${path}.tmp -Destination ${path} -Force
			} finally {
				$rd.Close()
				$wr.Close()
			}
		} else {
			throw [System.IO.FileNotFoundException]::new("could not find path: $path")
		}`, f.path))

	err := execute(f.ctx, f.shell, io.Discard, command)

	if cerr := f.shell.Close(); err == nil {
		err = cerr
	}

	return err
}

func (f *writableFile) Name() string {
	return f.path
}

func (f *writableFile) Write(p []byte) (int, error) {
	pb64 := base64.StdEncoding.EncodeToString(p)
	command := fmt.Sprintf(`echo %s >> %q`, pb64, f.path)

	f.err = execute(f.ctx, f.shell, io.Discard, command)
	if f.err != nil {
		return 0, f.err
	}

	return len(p), nil
}

type readonlyFile struct {
	ctx   context.Context
	shell *winrm.Shell
	path  string
}

func (f *readonlyFile) Close() error {
	return f.shell.Close()
}

func (f *readonlyFile) Name() string {
	return f.path
}

func (f *readonlyFile) Read(p []byte) (int, error) {
	// TODO(thxCode): implement this.
	return 0, io.EOF
}

func (f *readonlyFile) Stat() (fs.FileInfo, error) {
	command := winrm.Powershell(fmt.Sprintf("Get-ItemProperty -Path %s | "+
		"Select-Object -Property FullName,LastWriteTimeUtc,Attributes,Length | "+
		"ConvertTo-Xml -NoTypeInformation -As String", f.path))

	buf := bytespool.GetBuffer()
	defer func() { bytespool.Put(buf) }()

	err := execute(f.ctx, f.shell, buf, command)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %q: %w", f.path, err)
	}

	// <?xml version="1.0" encoding="utf-8"?>
	// <Objects>
	//  <Object>
	//    <Property Name="FullName">C:\Users\Administrator\test.txt</Property>
	//    <Property Name="LastWriteTimeUtc">2023/9/18 5:32:42</Property>
	//    <Property Name="Attributes">Archive</Property>
	//    <Property Name="Length">0</Property>
	//  </Object>
	// </Objects>.
	var r struct {
		Objects []struct {
			Properties []struct {
				Name  string `xml:"Name,attr"`
				Value string `xml:",chardata"`
			} `xml:"Property"`
		} `xml:"Object"`
	}

	err = xml.Unmarshal(buf.Bytes(), &r)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal file info: %w", err)
	}

	if len(r.Objects) != 1 || len(r.Objects[0].Properties) == 0 {
		return nil, errors.New("failed to unmarshal file info: no object or no properties")
	}

	var fi fileInfo
	for _, prop := range r.Objects[0].Properties {
		switch prop.Name {
		case "FullName":
			fi.name = prop.Value
		case "LastWriteTimeUtc":
			fi.modTime, err = time.Parse("2006/1/2 15:4:5", prop.Value)
		case "Attributes":
			if strings.Contains(prop.Value, "Directory") {
				fi.mode |= fs.ModeDir
			}
		case "Length":
			fi.size, err = strconv.ParseInt(prop.Value, 10, 64)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse file info: %w", err)
		}
	}

	return fi, nil
}

func toWindowsPath(path string) string {
	if strings.Contains(path, " ") {
		path = fmt.Sprintf("'%s'", strings.Trim(path, "'\""))
	}

	return strings.ReplaceAll(path, "/", "\\")
}

func execute(ctx context.Context, shell *winrm.Shell, out io.Writer, command string) error {
	c, err := shell.ExecuteWithContext(ctx, command)
	if err != nil {
		return err
	}

	defer func() { _ = c.Close() }()

	var g sync.WaitGroup

	cp := func(wr io.Writer, rd io.Reader) {
		defer g.Done()

		buf := bytespool.GetBytes()
		defer func() { bytespool.Put(buf) }()

		_, _ = io.CopyBuffer(wr, rd, buf)
	}

	stds := []io.Reader{c.Stdout, c.Stderr}
	for i := range stds {
		g.Add(1)
		go cp(out, stds[i])
	}

	g.Wait()

	c.Wait()
	if c.ExitCode() != 0 {
		return fmt.Errorf("exit %d", c.ExitCode())
	}

	return nil
}
