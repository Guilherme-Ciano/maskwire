package codec

import (
	"errors"
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
