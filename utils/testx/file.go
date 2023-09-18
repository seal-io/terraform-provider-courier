package testx

import (
	"os"
	"path/filepath"
)

func AbsolutePath(relativePath string) string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	return filepath.Join(dir, relativePath)
}

func File(relativePath string) (*os.File, error) {
	return os.Open(AbsolutePath(relativePath))
}
