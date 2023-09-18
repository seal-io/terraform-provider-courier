package iox

import (
	"io"
	"sync"
)

func SingleWriter(w io.Writer) io.Writer {
	return &singleWriter{
		Writer: w,
	}
}

type singleWriter struct {
	io.Writer

	m sync.Mutex
}

func (s *singleWriter) Write(p []byte) (n int, err error) {
	s.m.Lock()
	defer s.m.Unlock()
	return s.Writer.Write(p)
}
