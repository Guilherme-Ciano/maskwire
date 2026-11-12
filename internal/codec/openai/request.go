package openai

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

var ErrLogprobs = errors.New("logprobs are not supported through maskwire")

type Masker = codec.Masker

type RequestMeta = codec.Meta

type Codec struct {
	rw *jsonx.Rewriter
}

func New() *Codec {
	return &Codec{rw: jsonx.NewRewriter()}
}

func ParseMeta(body []byte) (RequestMeta, error) {
	var m RequestMeta
	if err := json.Unmarshal(body, &m, json.RejectUnknownMembers(false)); err != nil {
		return m, fmt.Errorf("%w: %w", codec.ErrInvalidBody, err)
	}
	return m, nil
}

const (
	kindText = "text"
	kindJSON = "json"
)

func skipKey(f jsonx.Frame) bool {
	return f.Is("type") || f.Is("image_url") || f.Is("input_audio") || f.Is("file") || f.Is("detail") || f.Is("format")
}

func requestTarget(path []jsonx.Frame) (string, bool) {
	if len(path) == 1 && path[0].Is("user") {
		return kindText, true
	}
	if len(path) < 3 || !path[0].Is("messages") || !path[1].Array {
		return "", false
	}
	field := path[2]
	switch {
	case field.Is("content"):
		for _, f := range path[3:] {
			if !f.Array && skipKey(f) {
				return "", false
			}
		}
		return kindText, true
	case field.Is("name"):
		return "text", len(path) == 3
	case field.Is("tool_calls"):
		if len(path) == 6 && path[4].Is("function") && path[5].Is("arguments") {
			return kindJSON, true
		}
	}
	return "", false
}

func (c *Codec) MaskRequest(dst, body []byte, m Masker) ([]byte, error) {
	var scratch, inner []byte
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		kind, ok := requestTarget(path)
		if !ok {
			return dst, false, nil
		}
		if kind == kindText {
			return m.MaskQuoted(dst, raw)
		}
		var err error
		scratch, err = jsonx.Unquote(scratch[:0], raw)
		if err != nil {
			return dst, false, err
		}
		if jsonx.Valid(scratch) {
			inner, err = m.MaskJSONStrings(inner[:0], scratch)
		} else {
			inner, err = m.MaskText(inner[:0], scratch)
		}
		if err != nil {
			return dst, false, err
		}
		if bytes.Equal(inner, scratch) {
			return dst, false, nil
		}
		return jsonx.Quote(dst, inner), true, nil
	})
	if err != nil {
		return out, fmt.Errorf("mask openai request: %w", err)
	}
	return out, nil
}
