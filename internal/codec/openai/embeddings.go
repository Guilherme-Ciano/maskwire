package openai

import (
	"fmt"
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

func embeddingTarget(path []jsonx.Frame, raw []byte) bool {
	if len(path) == 0 || !jsonx.IsString(raw) {
		return false
	}
	return path[0].Is("input") || (len(path) == 1 && path[0].Is("user"))
}

func (c *Codec) MaskEmbeddings(dst, body []byte, m codec.Masker) ([]byte, error) {
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		if !embeddingTarget(path, raw) {
			return dst, false, nil
		}
		return m.MaskQuoted(dst, raw)
	})
	if err != nil {
		return out, fmt.Errorf("mask embeddings request: %w", err)
	}
	return out, nil
}

type rawBody struct {
	io.ReadCloser
}

func (rawBody) Stats() restore.Stats {
	return restore.Stats{}
}

func PassStream(src io.ReadCloser) interface {
	io.ReadCloser
	Stats() restore.Stats
} {
	return rawBody{src}
}
