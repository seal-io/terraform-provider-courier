package osx

import (
	"fmt"
	"os"
)

func Exists(paths ...string) bool {
	if len(paths) == 0 {
		return false
	}

	for _, p := range paths {
		if !exists(p) {
			return false
		}
	}

	return true
}

func exists(path string) bool {
	if _, err := os.Lstat(path); err != nil {
		return !os.IsNotExist(err)
	}

	return true
}

func TempFile(pattern string) string {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		panic(fmt.Errorf("error creating temp file: %w", err))
	}

	defer func() { _ = f.Close() }()

	return f.Name()
}

func TempDir(pattern string) string {
	n, err := os.MkdirTemp("", pattern)
	if err != nil {
		panic(fmt.Errorf("error creating temp dir: %w", err))
	}

	return n
}
