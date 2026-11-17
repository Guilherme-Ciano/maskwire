package httpapi

import "sync"

const copyBufferSize = 32 << 10

type copyBuffers struct {
	pool sync.Pool
}

func (c *copyBuffers) Get() []byte {
	if b, ok := c.pool.Get().(*[]byte); ok {
		return *b
	}
	return make([]byte, copyBufferSize)
}

func (c *copyBuffers) Put(b []byte) {
	if cap(b) != copyBufferSize {
		return
	}
	b = b[:copyBufferSize]
	c.pool.Put(&b)
}
