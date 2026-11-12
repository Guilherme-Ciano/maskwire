package audit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

const (
	defaultBuffer = 4096
	batchMax      = 1000
	tailWindow    = 64 << 10
	dayLayout     = "2006-01-02"
)

var (
	ErrClosed      = errors.New("audit writer closed")
	ErrUnavailable = errors.New("audit log unavailable")
	ErrBadTenant   = errors.New("invalid tenant id for audit path")
)

type OnFull uint8

const (
	Block OnFull = iota
	Drop
)

type Options struct {
	Dir    string
	Buffer int
	OnFull OnFull
	Now    func() time.Time
}

type chain struct {
	day  string
	f    *os.File
	bw   *bufio.Writer
	prev [32]byte
}

type Writer struct {
	opts    Options
	ch      chan Event
	log     *slog.Logger
	chains  map[string]*chain
	buf     []byte
	written atomic.Uint64
	dropped atomic.Uint64
	failed  atomic.Bool
	closed  atomic.Bool
}

func NewWriter(o Options, log *slog.Logger) (*Writer, error) {
	if o.Buffer <= 0 {
		o.Buffer = defaultBuffer
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	return &Writer{opts: o, ch: make(chan Event, o.Buffer), log: log, chains: map[string]*chain{}}, nil
}

func (w *Writer) Written() uint64 {
	return w.written.Load()
}

func (w *Writer) Dropped() uint64 {
	return w.dropped.Load()
}

func (w *Writer) Healthy() bool {
	return !w.failed.Load()
}

func (w *Writer) Enqueue(ctx context.Context, e *Event) error {
	if w.closed.Load() {
		return ErrClosed
	}
	if w.failed.Load() {
		return ErrUnavailable
	}
	if w.opts.OnFull == Drop {
		select {
		case w.ch <- *e:
		default:
			w.dropped.Add(1)
		}
		return nil
	}
	select {
	case w.ch <- *e:
		return nil
	case <-ctx.Done():
		w.dropped.Add(1)
		return fmt.Errorf("enqueue audit event: %w", context.Cause(ctx))
	}
}

func (w *Writer) Run(ctx context.Context) error {
	defer w.closeAll()
	for {
		select {
		case <-ctx.Done():
			w.closed.Store(true)
			w.drain()
			return nil
		case e := <-w.ch:
			w.batch(e)
		}
	}
}

func (w *Writer) drain() {
	for {
		select {
		case e := <-w.ch:
			w.batch(e)
		default:
			return
		}
	}
}

func (w *Writer) batch(first Event) {
	touched := map[*chain]struct{}{}
	w.write(&first, touched)
	for range batchMax - 1 {
		select {
		case e := <-w.ch:
			w.write(&e, touched)
		default:
			w.sync(touched)
			return
		}
	}
	w.sync(touched)
}

func (w *Writer) fail(op string, err error) {
	w.failed.Store(true)
	w.log.Error("audit io failure", slog.String("op", op), slog.String("error", err.Error()))
}

func (w *Writer) write(e *Event, touched map[*chain]struct{}) {
	c, err := w.chainFor(e.Tenant, e.TS)
	if err != nil {
		w.fail("open", err)
		w.dropped.Add(1)
		return
	}
	w.buf = e.appendBody(w.buf[:0])
	var sum [32]byte
	w.buf, sum = seal(w.buf, &c.prev)
	if _, err := c.bw.Write(w.buf); err != nil {
		w.fail("write", err)
		w.dropped.Add(1)
		return
	}
	c.prev = sum
	touched[c] = struct{}{}
	w.written.Add(1)
}

func (w *Writer) sync(touched map[*chain]struct{}) {
	for c := range touched {
		if err := c.bw.Flush(); err != nil {
			w.fail("flush", err)
			continue
		}
		if err := c.f.Sync(); err != nil {
			w.fail("sync", err)
		}
	}
}

func validTenant(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 64 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if !tenantChar(c) {
			return false
		}
	}
	return true
}

func tenantChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' ||
		c == '.'
}

func FilePath(dir, tenant, day string) string {
	return filepath.Join(dir, tenant, day+".jsonl")
}

func (w *Writer) chainFor(tenant string, ts time.Time) (*chain, error) {
	if !validTenant(tenant) {
		return nil, ErrBadTenant
	}
	day := ts.UTC().Format(dayLayout)
	c, ok := w.chains[tenant]
	if ok && c.day == day {
		return c, nil
	}
	if ok {
		if err := c.bw.Flush(); err != nil {
			return nil, fmt.Errorf("flush audit file: %w", err)
		}
		if err := c.f.Close(); err != nil {
			return nil, fmt.Errorf("close audit file: %w", err)
		}
		delete(w.chains, tenant)
	}
	c, err := w.open(tenant, day)
	if err != nil {
		return nil, err
	}
	w.chains[tenant] = c
	return c, nil
}

func (w *Writer) open(tenant, day string) (*chain, error) {
	if err := os.MkdirAll(filepath.Join(w.opts.Dir, tenant), 0o750); err != nil {
		return nil, fmt.Errorf("create tenant audit dir: %w", err)
	}
	path := FilePath(w.opts.Dir, tenant, day)
	f, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit file: %w", err)
	}
	c := &chain{day: day, f: f, bw: bufio.NewWriterSize(f, 64<<10), prev: Genesis(tenant, day)}
	prev, torn, err := tail(f)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if prev != nil {
		c.prev = *prev
	}
	if torn > 0 {
		line, sum := seal(recoveryBody([]byte{'\n'}, w.opts.Now(), torn)[1:], &c.prev)
		if _, err := c.bw.Write(append([]byte{'\n'}, line...)); err != nil {
			return nil, errors.Join(fmt.Errorf("write recovery record: %w", err), f.Close())
		}
		c.prev = sum
	}
	return c, nil
}

func tail(f *os.File) (prev *[32]byte, torn int, err error) {
	st, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("stat audit file: %w", err)
	}
	size := st.Size()
	if size == 0 {
		return nil, 0, nil
	}
	off := max(size-tailWindow, 0)
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, fmt.Errorf("read audit tail: %w", err)
	}
	if buf[len(buf)-1] != '\n' {
		nl := bytes.LastIndexByte(buf, '\n')
		torn = len(buf) - nl - 1
		buf = buf[:nl+1]
	}
	if len(buf) == 0 {
		return nil, torn, nil
	}
	lines := bytes.Split(bytes.TrimSuffix(buf, []byte{'\n'}), []byte{'\n'})
	last := lines[len(lines)-1]
	sum, ok := lineHash(last)
	if !ok {
		return nil, torn, fmt.Errorf("audit file tail is not a sealed record: %w", ErrBrokenChain)
	}
	return &sum, torn, nil
}

func (w *Writer) closeAll() {
	for t, c := range w.chains {
		if err := c.bw.Flush(); err != nil {
			w.fail("flush", err)
		}
		if err := c.f.Sync(); err != nil {
			w.fail("sync", err)
		}
		if err := c.f.Close(); err != nil {
			w.fail("close", err)
		}
		delete(w.chains, t)
	}
}
