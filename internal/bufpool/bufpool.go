package bufpool

import (
	"bytes"
	"io"
	"math/bits"
	"sync"
	"sync/atomic"
)

const (
	minShift = 12
	maxShift = 26
	classes  = maxShift - minShift + 1
)

type Pool struct {
	classes [classes]sync.Pool
}

type Buf struct {
	B     []byte
	pool  *Pool
	class int
	refs  atomic.Int32
}

func class(n int) int {
	if n <= 1<<minShift {
		return 0
	}
	c := bits.Len(uint(n-1)) - minShift
	if c >= classes {
		return -1
	}
	return c
}

func (p *Pool) Get(n int) *Buf {
	c := class(n)
	if c < 0 {
		b := &Buf{B: make([]byte, 0, n), class: -1}
		b.refs.Store(1)
		return b
	}
	b, ok := p.classes[c].Get().(*Buf)
	if !ok {
		b = &Buf{B: make([]byte, 0, 1<<(c+minShift)), pool: p, class: c}
	}
	b.B = b.B[:0]
	b.refs.Store(1)
	return b
}

func (b *Buf) Retain() {
	b.refs.Add(1)
}

func (b *Buf) Release() {
	if b.refs.Add(-1) != 0 || b.class < 0 {
		return
	}
	clear(b.B[:cap(b.B)])
	b.B = b.B[:0]
	b.pool.classes[b.class].Put(b)
}

func (b *Buf) Reader() io.ReadCloser {
	b.Retain()
	return &reader{Reader: bytes.NewReader(b.B), buf: b}
}

type reader struct {
	*bytes.Reader
	buf  *Buf
	once sync.Once
}

func (r *reader) Close() error {
	r.once.Do(r.buf.Release)
	return nil
}

func Wrap(b []byte) *Buf {
	buf := &Buf{B: b, class: -1}
	buf.refs.Store(1)
	return buf
}
