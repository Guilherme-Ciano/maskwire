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
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBuffer = 4096
	batchMax      = 1000
	tailWindow    = 64 << 10
	dayLayout     = "2006-01-02"
	maxTenantLen  = 64
	probeEvery    = 5 * time.Second
	idleClose     = 2 * time.Minute
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
	Key    []byte
	Buffer int
	OnFull OnFull
	Now    func() time.Time

	AnchorPath    string
	RetentionDays int
}

type chain struct {
	tenant   string
	day      string
	f        *os.File
	bw       *bufio.Writer
	prev     [32]byte
	anchored [32]byte
	used     time.Time
}

type Writer struct {
	opts    Options
	sealer  *Sealer
	ch      chan Event
	log     *slog.Logger
	chains  map[string]*chain
	buf     []byte
	mu      sync.RWMutex
	closed  bool
	written atomic.Uint64
	dropped atomic.Uint64
	failed  atomic.Bool

	lastPrune time.Time
}

func NewWriter(o Options, log *slog.Logger) (*Writer, error) {
	if o.Buffer <= 0 {
		o.Buffer = defaultBuffer
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if len(o.Key) == 0 {
		return nil, errors.New("audit key is required")
	}
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	return &Writer{
		opts: o, sealer: NewSealer(o.Key), ch: make(chan Event, o.Buffer), log: log, chains: map[string]*chain{},
	}, nil
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
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		w.dropped.Add(1)
		return ErrClosed
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
	tick := time.NewTicker(probeEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			w.shutdown()
			return nil
		case e := <-w.ch:
			w.batch(e)
		case <-tick.C:
			w.maintain()
		}
	}
}

func (w *Writer) shutdown() {
	for {
		w.drain()
		if w.mu.TryLock() {
			w.closed = true
			w.mu.Unlock()
			w.drain()
			return
		}
		time.Sleep(time.Millisecond)
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
	if !ValidTenant(e.Tenant) {
		w.dropped.Add(1)
		w.log.Error("audit event dropped", slog.String("reason", "invalid tenant id"))
		return
	}
	c, err := w.chainFor(e.Tenant, e.TS)
	if err != nil {
		w.fail("open", err)
		w.dropped.Add(1)
		return
	}
	w.buf = e.appendBody(w.buf[:0])
	if err := w.append(c, w.buf); err != nil {
		w.fail("write", err)
		w.dropped.Add(1)
		return
	}
	touched[c] = struct{}{}
	w.written.Add(1)
}

func (w *Writer) append(c *chain, body []byte) error {
	line, sum := w.sealer.seal(body, &c.prev)
	if _, err := c.bw.Write(line); err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}
	c.prev = sum
	c.used = w.opts.Now()
	return nil
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

func (w *Writer) maintain() {
	w.anchor()
	w.prune()
	now := w.opts.Now()
	for key, c := range w.chains {
		if now.Sub(c.used) > idleClose {
			w.closeChain(c)
			delete(w.chains, key)
		}
	}
	if w.failed.Load() && w.probe() == nil {
		w.failed.Store(false)
		w.log.Info("audit log recovered")
	}
}

func (w *Writer) probe() error {
	for _, c := range w.chains {
		if err := errors.Join(c.bw.Flush(), c.f.Sync()); err != nil {
			return fmt.Errorf("probe chain: %w", err)
		}
	}
	f, err := os.CreateTemp(w.opts.Dir, ".probe-*")
	if err != nil {
		return fmt.Errorf("probe create: %w", err)
	}
	name := f.Name()
	return errors.Join(f.Sync(), f.Close(), os.Remove(name))
}

func ValidTenant(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > maxTenantLen {
		return false
	}
	for i := range len(id) {
		if !tenantChar(id[i]) {
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
	day := ts.UTC().Format(dayLayout)
	key := tenant + "/" + day
	if c, ok := w.chains[key]; ok {
		return c, nil
	}
	c, err := w.open(tenant, day)
	if err != nil {
		return nil, err
	}
	w.chains[key] = c
	return c, nil
}

func dayFiles(dir, tenant string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, tenant, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("list audit files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func dayOf(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

func previousDay(dir, tenant, day string) (prevDay string, head *[32]byte, err error) {
	files, err := dayFiles(dir, tenant)
	if err != nil {
		return "", nil, err
	}
	for _, path := range slices.Backward(files) {
		d := dayOf(path)
		if d >= day {
			continue
		}
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			return "", nil, fmt.Errorf("open previous audit file: %w", err)
		}
		h, _, terr := tail(f)
		if cerr := f.Close(); cerr != nil {
			return "", nil, fmt.Errorf("close previous audit file: %w", cerr)
		}
		if terr != nil {
			return "", nil, terr
		}
		return d, h, nil
	}
	return "", nil, nil
}

func (w *Writer) open(tenant, day string) (*chain, error) {
	if err := os.MkdirAll(filepath.Join(w.opts.Dir, tenant), 0o750); err != nil {
		return nil, fmt.Errorf("create tenant audit dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Clean(FilePath(w.opts.Dir, tenant, day)), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit file: %w", err)
	}
	c := &chain{tenant: tenant, day: day, f: f, bw: bufio.NewWriterSize(f, 64<<10), prev: w.sealer.Genesis(tenant, day)}
	prev, torn, err := tail(f)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err := w.start(c, prev, torn); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return c, nil
}

func (w *Writer) start(c *chain, prev *[32]byte, torn int) error {
	if prev != nil {
		c.prev = *prev
	}
	if torn > 0 {
		if err := c.bw.WriteByte('\n'); err != nil {
			return fmt.Errorf("terminate torn record: %w", err)
		}
		return w.append(c, recoveryBody(nil, w.opts.Now(), torn))
	}
	if prev != nil {
		return nil
	}
	prevDay, head, err := previousDay(w.opts.Dir, c.tenant, c.day)
	if err != nil {
		return err
	}
	return w.append(c, openBody(nil, w.opts.Now(), prevDay, head))
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
	sum, ok := lineHash(lines[len(lines)-1])
	if !ok {
		return nil, torn, fmt.Errorf("audit file tail is not a sealed record: %w", ErrBrokenChain)
	}
	return &sum, torn, nil
}

func (w *Writer) closeChain(c *chain) {
	if err := c.bw.Flush(); err != nil {
		w.fail("flush", err)
	}
	if err := c.f.Sync(); err != nil {
		w.fail("sync", err)
	}
	if err := c.f.Close(); err != nil {
		w.fail("close", err)
	}
}

func (w *Writer) closeAll() {
	w.anchor()
	for key, c := range w.chains {
		w.closeChain(c)
		delete(w.chains, key)
	}
}
