package httpapi

import (
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/anthropic"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/openai"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

type Meta = codec.Meta

type StreamBody interface {
	io.ReadCloser
	Stats() restore.Stats
}

type StreamOptions = codec.StreamOptions

type Codec interface {
	Name() string
	ParseMeta(body []byte) (Meta, error)
	MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error)
	RestoreBody(dst, body []byte, set restore.Set, o restore.Options) ([]byte, restore.Stats, error)
	NewStreamBody(src io.ReadCloser, set restore.Set, o StreamOptions) StreamBody
	Synthesize(dst, body []byte) ([]byte, error)
}

type openAICodec struct {
	c *openai.Codec
}

func NewOpenAICodec() Codec {
	return openAICodec{c: openai.New()}
}

func (openAICodec) Name() string {
	return "openai"
}

func (openAICodec) ParseMeta(body []byte) (Meta, error) {
	return openai.ParseMeta(body)
}

func (o openAICodec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	return o.c.MaskRequest(dst, body, m)
}

func (o openAICodec) RestoreBody(
	dst, body []byte,
	set restore.Set,
	opts restore.Options,
) ([]byte, restore.Stats, error) {
	return o.c.RestoreBody(dst, body, set, opts)
}

func (openAICodec) NewStreamBody(src io.ReadCloser, set restore.Set, o StreamOptions) StreamBody {
	return openai.NewStreamBody(src, set, o)
}

func (openAICodec) Synthesize(dst, body []byte) ([]byte, error) {
	return openai.Synthesize(dst, body)
}

type anthropicCodec struct {
	c *anthropic.Codec
}

func NewAnthropicCodec() Codec {
	return anthropicCodec{c: anthropic.New()}
}

func (anthropicCodec) Name() string {
	return "anthropic"
}

func (anthropicCodec) ParseMeta(body []byte) (Meta, error) {
	return anthropic.ParseMeta(body)
}

func (a anthropicCodec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	return a.c.MaskRequest(dst, body, m)
}

func (a anthropicCodec) RestoreBody(
	dst, body []byte,
	set restore.Set,
	o restore.Options,
) ([]byte, restore.Stats, error) {
	return a.c.RestoreBody(dst, body, set, o)
}

func (anthropicCodec) NewStreamBody(src io.ReadCloser, set restore.Set, o StreamOptions) StreamBody {
	return anthropic.NewStreamBody(src, set, o)
}

func (anthropicCodec) Synthesize(dst, body []byte) ([]byte, error) {
	return anthropic.Synthesize(dst, body)
}
