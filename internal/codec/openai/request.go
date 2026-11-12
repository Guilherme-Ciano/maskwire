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

func skipTop(name []byte) bool {
	switch string(name) {
	case "model", "response_format", "tool_choice", "parallel_tool_calls", "reasoning_effort", "service_tier",
		"modalities", "logit_bias", "stream_options", "audio", "function_call", "verbosity":
		return true
	}
	return false
}

func skipLeaf(name []byte) bool {
	switch string(name) {
	case "role",
		"type",
		"id",
		"tool_call_id",
		"detail",
		"format",
		"voice",
		"media_type",
		"file_id",
		"file_data",
		"data":
		return true
	}
	return false
}

func toolSchema(path []jsonx.Frame) bool {
	if len(path) < 2 || (!path[0].Is("tools") && !path[0].Is("functions")) {
		return false
	}
	return jsonx.Under(path, "parameters") || path[len(path)-1].Is("name")
}

func argumentsLeaf(path []jsonx.Frame) bool {
	n := len(path)
	return n >= 3 && path[0].Is("messages") && path[n-1].Is("arguments") && path[n-2].Is("function") ||
		n >= 3 && path[0].Is("messages") && path[n-1].Is("arguments") && path[n-2].Is("function_call")
}

func functionName(path []jsonx.Frame) bool {
	n := len(path)
	return n >= 2 && path[n-1].Is("name") && (path[n-2].Is("function") || path[n-2].Is("function_call"))
}

func requestTarget(path []jsonx.Frame, raw []byte) (string, bool) {
	if len(path) == 0 || !jsonx.IsString(raw) {
		return "", false
	}
	leaf := path[len(path)-1]
	switch {
	case skipTop(path[0].Name), toolSchema(path), functionName(path), jsonx.LooksBase64(raw):
		return "", false
	case !leaf.Array && skipLeaf(leaf.Name):
		return "", false
	case leaf.Is("url") && jsonx.HasPrefix(raw, "data:"):
		return "", false
	case argumentsLeaf(path):
		return kindJSON, true
	}
	return kindText, true
}

func (c *Codec) MaskRequest(dst, body []byte, m Masker) ([]byte, error) {
	var scratch, inner []byte
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		kind, ok := requestTarget(path, raw)
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
