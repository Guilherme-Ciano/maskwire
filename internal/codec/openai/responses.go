package openai

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/sse"
)

const (
	evTextDelta    = "response.output_text.delta"
	evTextDone     = "response.output_text.done"
	evRefusalDelta = "response.refusal.delta"
	evRefusalDone  = "response.refusal.done"
	evArgsDelta    = "response.function_call_arguments.delta"
	evArgsDone     = "response.function_call_arguments.done"
	evSummaryDelta = "response.reasoning_summary_text.delta"
	evSummaryDone  = "response.reasoning_summary_text.done"
	evCompleted    = "response.completed"
	evCreated      = "response.created"
	fieldArguments = "arguments"
	kindSummary    = "summary"
)

func responsesSkipLeaf(name []byte) bool {
	switch string(name) {
	case "type", "id", "call_id", "role", "status", "encrypted_content", "file_id", "file_data", "detail", "format":
		return true
	}
	return false
}

func responsesTopSkipped(name []byte) bool {
	switch string(name) {
	case "model", "tools", "tool_choice", "text", "reasoning", "previous_response_id", "store", "stream", "include",
		"truncation", "service_tier", "parallel_tool_calls", "max_output_tokens", "background":
		return true
	}
	return false
}

func responsesToolText(path []jsonx.Frame) bool {
	n := len(path)
	return n == 3 && path[0].Is("tools") && path[2].Is("description")
}

func responsesRequestTarget(path []jsonx.Frame, raw []byte) (string, bool) {
	if len(path) == 0 || !jsonx.IsString(raw) {
		return "", false
	}
	leaf := path[len(path)-1]
	if path[0].Is("text") || path[0].Is("tools") {
		if root := codec.SchemaRoot(path, "schema", "parameters"); root > 0 {
			if codec.SchemaText(path, root) {
				return kindText, true
			}
			return "", false
		}
		if path[0].Is("text") && leaf.Is("description") {
			return kindText, true
		}
	}
	switch {
	case responsesToolText(path):
		return kindText, true
	case responsesTopSkipped(path[0].Name), jsonx.LooksBase64(raw):
		return "", false
	case !leaf.Array && (responsesSkipLeaf(leaf.Name) || leaf.Is("name")):
		return "", false
	case leaf.Is("image_url") && jsonx.HasPrefix(raw, "data:"):
		return "", false
	case leaf.Is(fieldArguments):
		return kindJSON, true
	}
	return kindText, true
}

func (c *Codec) MaskResponses(dst, body []byte, m codec.Masker) ([]byte, error) {
	var scratch, inner []byte
	out, err := c.rw.Rewrite(dst, body, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		kind, ok := responsesRequestTarget(path, raw)
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
		return out, fmt.Errorf("mask responses request: %w", err)
	}
	return out, nil
}

func responsesRestoreTarget(path []jsonx.Frame, raw []byte) (restore.Encoding, bool) {
	if len(path) == 0 || !jsonx.IsString(raw) {
		return restore.Raw, false
	}
	leaf := path[len(path)-1]
	switch {
	case leaf.Is(fieldArguments):
		return restore.JSONString, true
	case leaf.Is("text"), leaf.Is("refusal"), leaf.Is("output"), leaf.Is("delta"):
		return restore.Raw, true
	}
	return restore.Raw, false
}

func (c *Codec) restoreLeaves(
	dst, doc []byte,
	set restore.Set,
	o restore.Options,
	stats *restore.Stats,
) ([]byte, error) {
	var scratch, restored []byte
	out, err := c.rw.Rewrite(dst, doc, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		enc, ok := responsesRestoreTarget(path, raw)
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
		var st restore.Stats
		restored, st = restore.Restore(restored[:0], scratch, set, opts)
		stats.Add(&st)
		if st.Restored == 0 && st.Foreign == 0 {
			return dst, false, nil
		}
		return jsonx.Quote(dst, restored), true, nil
	})
	if err != nil {
		return out, fmt.Errorf("restore responses document: %w", err)
	}
	return out, nil
}

func (c *Codec) RestoreResponses(dst, body []byte, set restore.Set, o restore.Options) ([]byte, restore.Stats, error) {
	var stats restore.Stats
	out, err := c.restoreLeaves(dst, body, set, o, &stats)
	return out, stats, err
}

type respEvent struct {
	Type         string         `json:"type"`
	ItemID       string         `json:"item_id,omitzero"`
	OutputIndex  *int           `json:"output_index,omitzero"`
	ContentIndex *int           `json:"content_index,omitzero"`
	SummaryIndex *int           `json:"summary_index,omitzero"`
	Delta        *string        `json:"delta,omitzero"`
	Extra        jsontext.Value `json:",embed"`
}

type respKey struct {
	item    string
	content int
	kind    string
}

type respChannel struct {
	key    respKey
	stream restore.Stream
}

type ResponsesStream struct {
	c        *Codec
	src      io.ReadCloser
	rd       *sse.Reader
	set      restore.Set
	opts     restore.Options
	channels []respChannel
	retired  restore.Stats
	whole    restore.Stats
	out      []byte
	pos      int
	scratch  []byte
	done     bool
	err      error
	idle     *codec.IdleReader
	events   int
	dropped  int
	onClose  func(restore.Stats, int, int)
}

func (c *Codec) NewResponsesStream(src io.ReadCloser, set restore.Set, o codec.StreamOptions) *ResponsesStream {
	idle := codec.WatchIdle(src, o.Idle, o.OnStall)
	return &ResponsesStream{
		c: c, src: src, rd: sse.NewReader(idle, o.MaxEvent), set: set, opts: o.Restore, idle: idle, onClose: o.OnClose,
	}
}

func (b *ResponsesStream) Stats() restore.Stats {
	s := b.retired
	s.Add(&b.whole)
	for i := range b.channels {
		s.Add(b.channels[i].stream.Stats())
	}
	return s
}

func (b *ResponsesStream) Read(p []byte) (int, error) {
	for b.pos >= len(b.out) {
		if b.done {
			return 0, b.err
		}
		b.out, b.pos = b.out[:0], 0
		b.step()
	}
	n := copy(p, b.out[b.pos:])
	b.pos += n
	return n, nil
}

func (b *ResponsesStream) Close() error {
	b.idle.Stop()
	err := b.src.Close()
	if b.onClose != nil {
		b.onClose(b.Stats(), b.events, b.dropped)
		b.onClose = nil
	}
	if err != nil {
		return fmt.Errorf("close upstream body: %w", err)
	}
	return nil
}

func (b *ResponsesStream) finish(err error) {
	b.done, b.err = true, err
	b.idle.Stop()
}

func deltaKind(t string) (kind string, ok bool) {
	switch t {
	case evTextDelta:
		return "text", true
	case evRefusalDelta:
		return "refusal", true
	case evArgsDelta:
		return fieldArguments, true
	case evSummaryDelta:
		return kindSummary, true
	}
	return "", false
}

func doneDelta(t string) (string, bool) {
	switch t {
	case evTextDone:
		return evTextDelta, true
	case evRefusalDone:
		return evRefusalDelta, true
	case evArgsDone:
		return evArgsDelta, true
	case evSummaryDone:
		return evSummaryDelta, true
	}
	return "", false
}

func indexOf(e *respEvent) int {
	switch {
	case e.ContentIndex != nil:
		return *e.ContentIndex
	case e.SummaryIndex != nil:
		return *e.SummaryIndex
	}
	return 0
}

func (b *ResponsesStream) channel(k respKey) (*restore.Stream, error) {
	for i := range b.channels {
		if b.channels[i].key == k {
			return &b.channels[i].stream, nil
		}
	}
	if len(b.channels) >= codec.MaxChannels {
		return nil, codec.ErrTooManyChannels
	}
	o := b.opts
	o.Encoding = restore.Raw
	if k.kind == fieldArguments {
		o.Encoding = restore.JSONString
	}
	b.channels = append(b.channels, respChannel{key: k, stream: restore.NewStream(b.set, o)})
	return &b.channels[len(b.channels)-1].stream, nil
}

func (b *ResponsesStream) step() {
	ev, err := b.rd.Next()
	if errors.Is(err, io.EOF) {
		b.flushAll()
		b.finish(io.EOF)
		return
	}
	if err != nil {
		b.finish(fmt.Errorf("stream restore: %w", err))
		return
	}
	b.events++
	if ev.Comment || !ev.HasData {
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	var e respEvent
	if err := json.Unmarshal(ev.Data, &e); err != nil {
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	data, keep, err := b.rewrite(&e, ev.Data)
	if err != nil {
		b.finish(err)
		return
	}
	if !keep {
		b.dropped++
		return
	}
	ev.Data = data
	b.out = sse.AppendEvent(b.out, ev)
}

func (b *ResponsesStream) rewrite(e *respEvent, raw []byte) (data []byte, keep bool, err error) {
	if kind, ok := deltaKind(e.Type); ok && e.Delta != nil {
		st, err := b.channel(respKey{e.ItemID, indexOf(e), kind})
		if err != nil {
			return nil, false, err
		}
		b.scratch = st.Feed(b.scratch[:0], []byte(*e.Delta))
		if len(b.scratch) == 0 {
			return nil, false, nil
		}
		text := string(b.scratch)
		e.Delta = &text
		out, err := json.Marshal(e)
		if err != nil {
			return nil, false, fmt.Errorf("encode responses delta: %w", err)
		}
		return out, true, nil
	}
	if deltaType, ok := doneDelta(e.Type); ok {
		kind, _ := deltaKind(deltaType)
		b.flushChannel(respKey{e.ItemID, indexOf(e), kind}, deltaType, e)
	}
	if e.Type == evCompleted {
		b.flushAll()
	}
	out, err := b.c.restoreLeaves(nil, raw, b.set, b.opts, &b.whole)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (b *ResponsesStream) flushChannel(k respKey, deltaType string, src *respEvent) {
	for i := range b.channels {
		c := &b.channels[i]
		if c.key != k || c.stream.Held() == 0 {
			continue
		}
		b.scratch = c.stream.Flush(b.scratch[:0])
		text := string(b.scratch)
		synth := respEvent{Type: deltaType, ItemID: k.item, OutputIndex: src.OutputIndex, Delta: &text}
		idx := k.content
		if k.kind == kindSummary {
			synth.SummaryIndex = &idx
		} else if k.kind != fieldArguments {
			synth.ContentIndex = &idx
		}
		out, err := json.Marshal(&synth)
		if err != nil {
			continue
		}
		b.out = sse.AppendEvent(b.out, sse.Event{Name: []byte(deltaType), Data: out, HasData: true})
	}
}

func (b *ResponsesStream) flushAll() {
	for i := range b.channels {
		c := &b.channels[i]
		if c.stream.Held() == 0 {
			continue
		}
		deltaType := evTextDelta
		switch c.key.kind {
		case "refusal":
			deltaType = evRefusalDelta
		case fieldArguments:
			deltaType = evArgsDelta
		case kindSummary:
			deltaType = evSummaryDelta
		}
		b.flushChannel(c.key, deltaType, &respEvent{})
	}
}

type respBody struct {
	Output []respItem `json:"output"`
}

type respItem struct {
	Type    string        `json:"type"`
	ID      string        `json:"id"`
	Content []respContent `json:"content"`
}

type respContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type synthRespEvent struct {
	Type         string         `json:"type"`
	ItemID       string         `json:"item_id,omitzero"`
	OutputIndex  *int           `json:"output_index,omitzero"`
	ContentIndex *int           `json:"content_index,omitzero"`
	Delta        *string        `json:"delta,omitzero"`
	Text         *string        `json:"text,omitzero"`
	Response     jsontext.Value `json:"response,omitzero"`
}

func appendRespEvent(dst []byte, e *synthRespEvent) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return dst, fmt.Errorf("synthesize responses stream: %w", err)
	}
	return sse.AppendEvent(dst, sse.Event{Name: []byte(e.Type), Data: b, HasData: true}), nil
}

func SynthesizeResponses(dst, body []byte) ([]byte, error) {
	var r respBody
	if err := json.Unmarshal(body, &r); err != nil {
		return dst, fmt.Errorf("synthesize responses stream: %w", err)
	}
	var err error
	if dst, err = appendRespEvent(dst, &synthRespEvent{Type: evCreated, Response: jsontext.Value(body)}); err != nil {
		return dst, err
	}
	for oi, item := range r.Output {
		for ci, part := range item.Content {
			if part.Type != "output_text" {
				continue
			}
			o, c, text := oi, ci, part.Text
			events := []synthRespEvent{
				{Type: evTextDelta, ItemID: item.ID, OutputIndex: &o, ContentIndex: &c, Delta: &text},
				{Type: evTextDone, ItemID: item.ID, OutputIndex: &o, ContentIndex: &c, Text: &text},
			}
			for k := range events {
				if dst, err = appendRespEvent(dst, &events[k]); err != nil {
					return dst, err
				}
			}
		}
	}
	return appendRespEvent(dst, &synthRespEvent{Type: evCompleted, Response: jsontext.Value(body)})
}
