package runtime

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

type Classes map[string]map[string]struct{}

func (r Classes) Has(rt string) bool {
	_, ok := r[rt]
	return ok
}

func (r Classes) HasOS(rt, os string) bool {
	if !r.Has(rt) {
		return false
	}

	_, ok := r[rt][os]
	return ok
}

func GetClasses(dir Source) (Classes, error) {
	di, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to get runtimes list: %w",
			err)
	}

	clss := make(Classes, len(di))

	for i := range di {
		dj, err := fs.ReadDir(dir, di[i].Name())
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get os list of runtime %s: %w",
				di[i].Name(),
				err,
			)
		}

		if len(dj) == 0 {
			continue
		}

		rt := filepath.Base(di[i].Name())

		if rt == "lib" {
			continue
		}

		clss[rt] = map[string]struct{}{}

		for j := range dj {
			os := filepath.Base(dj[j].Name())

			clss[rt][os] = struct{}{}
		}
	}

	return clss, nil
}
