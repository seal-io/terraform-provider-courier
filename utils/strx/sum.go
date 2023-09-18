package strx

import (
	"encoding/hex"
	"hash/fnv"
)

func Sum(ss ...string) string {
	if len(ss) == 0 {
		return ""
	}

	h := fnv.New64a()
	for i := range ss {
		_, _ = h.Write([]byte(ss[i]))
	}

	return hex.EncodeToString(h.Sum(nil))
}
