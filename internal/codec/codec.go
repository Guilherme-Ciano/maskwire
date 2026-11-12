package codec

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

var (
	ErrUpstreamStalled = errors.New("upstream stream stalled")
	ErrInvalidBody     = errors.New("invalid request body")
	ErrTooManyChannels = errors.New("too many stream channels")
)

const MaxChannels = 256

type Masker interface {
	MaskText(dst, src []byte) ([]byte, error)
	MaskQuoted(dst, raw []byte) ([]byte, bool, error)
	MaskLeaf(dst, raw []byte) ([]byte, bool, error)
	MaskJSONStrings(dst, doc []byte) ([]byte, error)
}

type StreamOptions struct {
	Restore  restore.Options
	Idle     time.Duration
	MaxEvent int
	OnStall  func()
	OnClose  func(stats restore.Stats, events, dropped int)
}

type Meta struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Logprobs bool   `json:"logprobs"`
}

type IdleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func WatchIdle(r io.Reader, idle time.Duration, onStall func()) *IdleReader {
	ir := &IdleReader{r: r, idle: idle}
	if idle > 0 && onStall != nil {
		ir.timer = time.AfterFunc(idle, onStall)
		ir.timer.Stop()
	}
	return ir
}

func (r *IdleReader) Read(p []byte) (int, error) {
	if r.timer == nil {
		return r.read(p)
	}
	r.timer.Reset(r.idle)
	n, err := r.read(p)
	r.timer.Stop()
	return n, err
}

func (r *IdleReader) read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil {
		return n, fmt.Errorf("read upstream: %w", err)
	}
	return n, nil
}

func (r *IdleReader) Stop() {
	if r.timer != nil {
		r.timer.Stop()
	}
}
