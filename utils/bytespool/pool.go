package bytespool

import (
	"sync"

	"github.com/valyala/bytebufferpool"
)

const defaultBytesSliceSize = 32 * 1024

var gp = sync.Pool{
	New: func() any {
		bs := make([]byte, defaultBytesSliceSize)
		return &bs
	},
}

func GetBuffer() *bytebufferpool.ByteBuffer {
	return bytebufferpool.Get()
}

func GetBytes(size ...int) []byte {
	var (
		bsp = gp.Get().(*[]byte)
		bs  = *bsp
		l   int
	)

	if len(size) > 0 {
		l = size[len(size)-1]
	}

	if l <= 0 {
		l = defaultBytesSliceSize
	}

	if cap(bs) >= l {
		return bs[:l]
	}

	gp.Put(bsp)

	return make([]byte, l)
}

func Put(b any) {
	switch t := b.(type) {
	case []byte:
		gp.Put(&t)
	case *bytebufferpool.ByteBuffer:
		bytebufferpool.Put(t)
	}
}
