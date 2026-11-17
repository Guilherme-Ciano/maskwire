package httpapi

import (
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/anthropic"
	"github.com/Guilherme-Ciano/maskwire/internal/codec/gemini"
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
	InjectHint(body []byte) ([]byte, error)
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

type embeddingsCodec struct {
	c *openai.Codec
}

func NewEmbeddingsCodec() Codec {
	return embeddingsCodec{c: openai.New()}
}

func (embeddingsCodec) Name() string {
	return "openai-embeddings"
}

func (embeddingsCodec) ParseMeta(body []byte) (Meta, error) {
	m, err := openai.ParseMeta(body)
	m.Stream, m.Logprobs = false, false
	return m, err
}

func (e embeddingsCodec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	return e.c.MaskEmbeddings(dst, body, m)
}

func (embeddingsCodec) RestoreBody(dst, body []byte, _ restore.Set, _ restore.Options) ([]byte, restore.Stats, error) {
	return append(dst, body...), restore.Stats{}, nil
}

func (embeddingsCodec) NewStreamBody(src io.ReadCloser, _ restore.Set, _ StreamOptions) StreamBody {
	return openai.PassStream(src)
}

func (embeddingsCodec) Synthesize(dst, body []byte) ([]byte, error) {
	return append(dst, body...), nil
}

type responsesCodec struct {
	c *openai.Codec
}

func NewResponsesCodec() Codec {
	return responsesCodec{c: openai.New()}
}

func (responsesCodec) Name() string {
	return "openai-responses"
}

func (responsesCodec) ParseMeta(body []byte) (Meta, error) {
	m, err := openai.ParseMeta(body)
	m.Logprobs = false
	return m, err
}

func (r responsesCodec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	return r.c.MaskResponses(dst, body, m)
}

func (r responsesCodec) RestoreBody(
	dst, body []byte,
	set restore.Set,
	o restore.Options,
) ([]byte, restore.Stats, error) {
	return r.c.RestoreResponses(dst, body, set, o)
}

func (r responsesCodec) NewStreamBody(src io.ReadCloser, set restore.Set, o StreamOptions) StreamBody {
	return r.c.NewResponsesStream(src, set, o)
}

func (responsesCodec) Synthesize(dst, body []byte) ([]byte, error) {
	return openai.SynthesizeResponses(dst, body)
}

func (openAICodec) InjectHint(body []byte) ([]byte, error) {
	return codec.InjectOpenAIHint(body)
}

func (anthropicCodec) InjectHint(body []byte) ([]byte, error) {
	return codec.InjectStringHint(body, "system")
}

func (responsesCodec) InjectHint(body []byte) ([]byte, error) {
	return codec.InjectStringHint(body, "instructions")
}

func (embeddingsCodec) InjectHint(body []byte) ([]byte, error) {
	return body, nil
}

type geminiCodec struct {
	c *gemini.Codec
}

func NewGeminiCodec() Codec {
	return geminiCodec{c: gemini.New()}
}

func (geminiCodec) Name() string {
	return "gemini"
}

func (geminiCodec) ParseMeta(body []byte) (Meta, error) {
	return gemini.ParseMeta(body)
}

func (g geminiCodec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	return g.c.MaskRequest(dst, body, m)
}

func (g geminiCodec) RestoreBody(
	dst, body []byte,
	set restore.Set,
	o restore.Options,
) ([]byte, restore.Stats, error) {
	return g.c.RestoreBody(dst, body, set, o)
}

func (geminiCodec) NewStreamBody(src io.ReadCloser, set restore.Set, o StreamOptions) StreamBody {
	return gemini.NewStreamBody(src, set, o)
}

func (geminiCodec) Synthesize(dst, body []byte) ([]byte, error) {
	return gemini.Synthesize(dst, body)
}

func (geminiCodec) InjectHint(body []byte) ([]byte, error) {
	return gemini.InjectHint(body)
}
