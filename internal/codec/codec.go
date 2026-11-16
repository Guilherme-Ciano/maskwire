package codec

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
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

func SchemaText(path []jsonx.Frame, from int) bool {
	if from >= len(path) {
		return false
	}
	for _, f := range path[from:] {
		if f.Is("enum") || f.Is("examples") {
			return true
		}
	}
	for i := len(path) - 1; i >= from; i-- {
		if path[i].Array {
			continue
		}
		return path[i].Is("description") || path[i].Is("title") || path[i].Is("default") || path[i].Is("const")
	}
	return false
}

func SchemaRoot(path []jsonx.Frame, names ...string) int {
	for i, f := range path {
		if slices.ContainsFunc(names, f.Is) {
			return i + 1
		}
	}
	return -1
}

const PromptHint = "Text such as [EMAIL_K7Q2MA] or [CPF_A0SA1Q] is a placeholder for personal data. " +
	"Copy each placeholder exactly as written, brackets included, and never translate, split, reformat or invent one."

func InjectOpenAIHint(doc []byte) ([]byte, error) {
	start, end, ok, err := jsonx.TopLevel(doc, "messages")
	if err != nil || !ok || doc[start] != '[' {
		return doc, err
	}
	msg := []byte(`{"role":"system","content":`)
	msg = jsonx.Quote(msg, []byte(PromptHint))
	return jsonx.PrependToArray(doc, start, end, append(msg, '}')), nil
}

func InjectStringHint(doc []byte, key string) ([]byte, error) {
	start, end, ok, err := jsonx.TopLevel(doc, key)
	if err != nil {
		return doc, err
	}
	if !ok {
		return jsonx.InsertTopLevel(doc, key, jsonx.Quote(nil, []byte(PromptHint)))
	}
	switch doc[start] {
	case '"':
		return jsonx.PrependToString(doc, start, end, PromptHint+"\n\n")
	case '[':
		block := []byte(`{"type":"text","text":`)
		block = jsonx.Quote(block, []byte(PromptHint))
		return jsonx.PrependToArray(doc, start, end, append(block, '}')), nil
	}
	return doc, nil
}
