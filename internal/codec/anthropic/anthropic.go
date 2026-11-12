package anthropic

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

type Codec struct {
	rw *jsonx.Rewriter
}

func New() *Codec {
	return &Codec{rw: jsonx.NewRewriter()}
}

func ParseMeta(body []byte) (codec.Meta, error) {
	var m struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return codec.Meta{}, fmt.Errorf("%w: %w", codec.ErrInvalidBody, err)
	}
	return codec.Meta{Model: m.Model, Stream: m.Stream}, nil
}

func protected(f jsonx.Frame) bool {
	return f.Is("thinking") || f.Is("signature") || f.Is("data") || f.Is("source") || f.Is("type") ||
		f.Is("id") || f.Is("tool_use_id") || f.Is("name") || f.Is("cache_control") || f.Is("citations")
}

func blockText(path []jsonx.Frame) bool {
	if len(path) == 0 {
		return false
	}
	for _, f := range path {
		if !f.Array && protected(f) {
			return false
		}
	}
	leaf := path[len(path)-1]
	return leaf.Array || leaf.Is("text") || leaf.Is("content")
}

func underInput(path []jsonx.Frame) bool {
	for _, f := range path {
		if !f.Array && f.Is("input") {
			return true
		}
	}
	return false
}

func requestTarget(path []jsonx.Frame) bool {
	switch {
	case len(path) == 1 && path[0].Is("system"):
		return true
	case len(path) >= 1 && path[0].Is("system"):
		return blockText(path[1:])
	case len(path) == 2 && path[0].Is("metadata") && path[1].Is("user_id"):
		return true
	case len(path) >= 3 && path[0].Is("messages") && path[2].Is("content"):
		rest := path[2:]
		if underInput(rest) {
			return true
		}
		return blockText(rest)
	}
	return false
}

func (c *Codec) MaskRequest(dst, body []byte, m codec.Masker) ([]byte, error) {
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		if !requestTarget(path) {
			return dst, false, nil
		}
		return m.MaskQuoted(dst, raw)
	})
	if err != nil {
		return out, fmt.Errorf("mask anthropic request: %w", err)
	}
	return out, nil
}

func responseTarget(path []jsonx.Frame) bool {
	if len(path) < 3 || !path[0].Is("content") {
		return false
	}
	return (len(path) == 3 && path[2].Is("text")) || underInput(path[2:])
}

func (c *Codec) RestoreBody(dst, body []byte, set restore.Set, o restore.Options) ([]byte, restore.Stats, error) {
	var stats restore.Stats
	var scratch, restored []byte
	o.Encoding = restore.Raw
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		if !responseTarget(path) {
			return dst, false, nil
		}
		var err error
		scratch, err = jsonx.Unquote(scratch[:0], raw)
		if err != nil {
			return dst, false, err
		}
		var st restore.Stats
		restored, st = restore.Restore(restored[:0], scratch, set, o)
		stats.Add(&st)
		if st.Restored == 0 && st.Foreign == 0 {
			return dst, false, nil
		}
		return jsonx.Quote(dst, restored), true, nil
	})
	if err != nil {
		return out, stats, fmt.Errorf("restore anthropic body: %w", err)
	}
	return out, stats, nil
}

type message struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []block        `json:"content"`
	StopReason   jsontext.Value `json:"stop_reason,omitzero"`
	StopSequence jsontext.Value `json:"stop_sequence,omitzero"`
	Usage        jsontext.Value `json:"usage,omitzero"`
	Extra        jsontext.Value `json:",unknown"`
}

type block struct {
	Type  string         `json:"type"`
	Text  *string        `json:"text,omitzero"`
	Input jsontext.Value `json:"input,omitzero"`
	Extra jsontext.Value `json:",unknown"`
}

const (
	evMessageStart = "message_start"
	evMessageDelta = "message_delta"
	evMessageStop  = "message_stop"
	evBlockStart   = "content_block_start"
	evBlockDelta   = "content_block_delta"
	evBlockStop    = "content_block_stop"
	blockTypeText  = "text"
	blockTypeTool  = "tool_use"
	deltaText      = "text_delta"
	deltaJSON      = "input_json_delta"
)

type synthDelta struct {
	Type         string         `json:"type,omitempty"`
	Text         *string        `json:"text,omitzero"`
	PartialJSON  *string        `json:"partial_json,omitzero"`
	StopReason   jsontext.Value `json:"stop_reason,omitzero"`
	StopSequence jsontext.Value `json:"stop_sequence,omitzero"`
}

type synthEvent struct {
	Type         string         `json:"type"`
	Index        *int           `json:"index,omitzero"`
	Message      *message       `json:"message,omitzero"`
	ContentBlock jsontext.Value `json:"content_block,omitzero"`
	Delta        *synthDelta    `json:"delta,omitzero"`
	Usage        jsontext.Value `json:"usage,omitzero"`
}

func appendSSE(dst []byte, ev *synthEvent) ([]byte, error) {
	b, err := json.Marshal(ev)
	if err != nil {
		return dst, fmt.Errorf("synthesize anthropic stream: %w", err)
	}
	dst = append(dst, "event: "...)
	dst = append(dst, ev.Type...)
	dst = append(dst, "\ndata: "...)
	dst = append(dst, b...)
	return append(dst, '\n', '\n'), nil
}

func startBlockFor(b *block) (start jsontext.Value, d *synthDelta, err error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, nil, fmt.Errorf("encode content block: %w", err)
	}
	switch {
	case b.Type == blockTypeText && b.Text != nil:
		empty := ""
		text := *b.Text
		shell := *b
		shell.Text = &empty
		start, err = json.Marshal(&shell)
		return start, &synthDelta{Type: deltaText, Text: &text}, wrapEncode(err)
	case b.Type == blockTypeTool:
		args := string(b.Input)
		if args == "" {
			args = "{}"
		}
		shell := *b
		shell.Input = jsontext.Value("{}")
		start, err = json.Marshal(&shell)
		return start, &synthDelta{Type: deltaJSON, PartialJSON: &args}, wrapEncode(err)
	}
	return raw, nil, nil
}

func wrapEncode(err error) error {
	if err != nil {
		return fmt.Errorf("encode content block: %w", err)
	}
	return nil
}

func Synthesize(dst, body []byte) ([]byte, error) {
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return dst, fmt.Errorf("synthesize anthropic stream: %w", err)
	}
	start := m
	start.Content = []block{}
	start.StopReason, start.StopSequence = jsontext.Value("null"), jsontext.Value("null")
	dst, err := appendSSE(dst, &synthEvent{Type: evMessageStart, Message: &start})
	if err != nil {
		return dst, err
	}
	for i := range m.Content {
		idx := i
		cb, d, err := startBlockFor(&m.Content[i])
		if err != nil {
			return dst, err
		}
		events := []synthEvent{{Type: evBlockStart, Index: &idx, ContentBlock: cb}}
		if d != nil {
			events = append(events, synthEvent{Type: evBlockDelta, Index: &idx, Delta: d})
		}
		events = append(events, synthEvent{Type: evBlockStop, Index: &idx})
		for k := range events {
			if dst, err = appendSSE(dst, &events[k]); err != nil {
				return dst, err
			}
		}
	}
	md := synthEvent{
		Type:  evMessageDelta,
		Delta: &synthDelta{StopReason: m.StopReason, StopSequence: m.StopSequence},
		Usage: m.Usage,
	}
	if dst, err = appendSSE(dst, &md); err != nil {
		return dst, err
	}
	return appendSSE(dst, &synthEvent{Type: evMessageStop})
}
