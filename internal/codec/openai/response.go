package openai

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

func responseTarget(path []jsonx.Frame, raw []byte) (restore.Encoding, bool) {
	if !jsonx.IsString(raw) || len(path) < 4 || !path[0].Is("choices") || !path[2].Is("message") {
		return restore.Raw, false
	}
	switch {
	case len(path) == 4 && (path[3].Is("content") || path[3].Is("refusal")):
		return restore.Raw, true
	case len(path) == 7 && path[3].Is("tool_calls") && path[5].Is("function") && path[6].Is("arguments"):
		return restore.JSONString, true
	}
	return restore.Raw, false
}

func (c *Codec) RestoreBody(dst, body []byte, set restore.Set, o restore.Options) ([]byte, restore.Stats, error) {
	var stats restore.Stats
	var scratch, restored []byte
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		enc, ok := responseTarget(path, raw)
		if !ok {
			return dst, false, nil
		}
		var err error
		scratch, err = jsonx.Unquote(scratch[:0], raw)
		if err != nil {
			return dst, false, err
		}
		opts := o
		opts.Encoding = enc
		opts.Tool = enc == restore.JSONString
		var st restore.Stats
		restored, st = restore.Restore(restored[:0], scratch, set, opts)
		stats.Add(&st)
		if st.Restored == 0 && st.Foreign == 0 {
			return dst, false, nil
		}
		return jsonx.Quote(dst, restored), true, nil
	})
	if err != nil {
		return out, stats, fmt.Errorf("restore openai body: %w", err)
	}
	return out, stats, nil
}

type completion struct {
	ID      string         `json:"id"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []fullChoice   `json:"choices"`
	Usage   jsontext.Value `json:"usage,omitzero"`
	Extra   jsontext.Value `json:",embed"`
}

type fullChoice struct {
	Index        int            `json:"index"`
	Message      fullMessage    `json:"message"`
	FinishReason *string        `json:"finish_reason"`
	Extra        jsontext.Value `json:",embed"`
}

type fullMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content"`
	Refusal   *string          `json:"refusal,omitzero"`
	ToolCalls []jsontext.Value `json:"tool_calls,omitzero"`
}

type synthChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []synthChoice  `json:"choices"`
	Usage   jsontext.Value `json:"usage,omitzero"`
}

type synthChoice struct {
	Index        int        `json:"index"`
	Delta        synthDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type synthDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   *string          `json:"content,omitzero"`
	Refusal   *string          `json:"refusal,omitzero"`
	ToolCalls []jsontext.Value `json:"tool_calls,omitzero"`
}

func toolCallsWithIndex(calls []jsontext.Value) ([]jsontext.Value, error) {
	out := make([]jsontext.Value, 0, len(calls))
	for i, raw := range calls {
		var m map[string]jsontext.Value
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("decode tool call: %w", err)
		}
		idx, err := json.Marshal(i)
		if err != nil {
			return nil, fmt.Errorf("encode tool index: %w", err)
		}
		m["index"] = idx
		v, err := json.Marshal(m, json.Deterministic(true))
		if err != nil {
			return nil, fmt.Errorf("encode tool call: %w", err)
		}
		out = append(out, v)
	}
	return out, nil
}

func Synthesize(dst, body []byte) ([]byte, error) {
	var c completion
	if err := json.Unmarshal(body, &c); err != nil {
		return dst, fmt.Errorf("synthesize stream: %w", err)
	}
	emit := func(dst []byte, ch synthChunk) ([]byte, error) {
		ch.ID, ch.Object, ch.Created, ch.Model = c.ID, "chat.completion.chunk", c.Created, c.Model
		b, err := json.Marshal(ch)
		if err != nil {
			return dst, fmt.Errorf("synthesize stream: %w", err)
		}
		dst = append(dst, "data: "...)
		dst = append(dst, b...)
		return append(dst, '\n', '\n'), nil
	}
	var err error
	for _, choice := range c.Choices {
		role := choice.Message.Role
		if role == "" {
			role = "assistant"
		}
		calls, cerr := toolCallsWithIndex(choice.Message.ToolCalls)
		if cerr != nil {
			return dst, cerr
		}
		delta := synthDelta{
			Role:      role,
			Content:   choice.Message.Content,
			Refusal:   choice.Message.Refusal,
			ToolCalls: calls,
		}
		if dst, err = emit(dst, synthChunk{Choices: []synthChoice{{Index: choice.Index, Delta: delta}}}); err != nil {
			return dst, err
		}
		if dst, err = emit(
			dst,
			synthChunk{Choices: []synthChoice{{Index: choice.Index, FinishReason: choice.FinishReason}}},
		); err != nil {
			return dst, err
		}
	}
	if len(c.Usage) > 0 && string(c.Usage) != jsonNull {
		if dst, err = emit(dst, synthChunk{Choices: []synthChoice{}, Usage: c.Usage}); err != nil {
			return dst, err
		}
	}
	return append(dst, "data: [DONE]\n\n"...), nil
}
