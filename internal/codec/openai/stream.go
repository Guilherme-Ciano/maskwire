package openai

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/sse"
)

const (
	kindContent = 0
	kindRefusal = 1
	kindTool    = 2
)

type sChunk struct {
	Choices []sChoice      `json:"choices"`
	Extra   jsontext.Value `json:",embed"`
}

type sChoice struct {
	Index        int            `json:"index"`
	Delta        *sDelta        `json:"delta,omitzero"`
	FinishReason *string        `json:"finish_reason"`
	Extra        jsontext.Value `json:",embed"`
}

type sDelta struct {
	Content   *string        `json:"content,omitzero"`
	Refusal   *string        `json:"refusal,omitzero"`
	ToolCalls []sTool        `json:"tool_calls,omitzero"`
	Extra     jsontext.Value `json:",embed"`
}

type sTool struct {
	Index    int            `json:"index"`
	Function *sFunction     `json:"function,omitzero"`
	Extra    jsontext.Value `json:",embed"`
}

type sFunction struct {
	Arguments *string        `json:"arguments,omitzero"`
	Extra     jsontext.Value `json:",embed"`
}

type channelKey struct {
	choice int
	kind   int
	tool   int
}

type channel struct {
	key    channelKey
	stream restore.Stream
}

type StreamBody struct {
	src      io.ReadCloser
	rd       *sse.Reader
	set      restore.Set
	opts     restore.Options
	channels []channel
	out      []byte
	pos      int
	scratch  []byte
	done     bool
	err      error
	idle     *codec.IdleReader
	header   jsontext.Value
	changed  bool
	events   int
	dropped  int
	onClose  func(restore.Stats, int, int)
}

func NewStreamBody(src io.ReadCloser, set restore.Set, o codec.StreamOptions) *StreamBody {
	idle := codec.WatchIdle(src, o.Idle, o.OnStall)
	return &StreamBody{
		src:     src,
		rd:      sse.NewReader(idle, o.MaxEvent),
		set:     set,
		opts:    o.Restore,
		idle:    idle,
		onClose: o.OnClose,
	}
}

func (b *StreamBody) Stats() restore.Stats {
	var s restore.Stats
	for i := range b.channels {
		s.Add(b.channels[i].stream.Stats())
	}
	return s
}

func (b *StreamBody) Events() (events, dropped int) {
	return b.events, b.dropped
}

func (b *StreamBody) Read(p []byte) (int, error) {
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

func (b *StreamBody) Close() error {
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

func (b *StreamBody) finish(err error) {
	b.done = true
	b.err = err
	b.idle.Stop()
}

func (b *StreamBody) step() {
	ev, err := b.rd.Next()
	if errors.Is(err, io.EOF) {
		b.out = b.flushAll(b.out)
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
	if bytes.Equal(bytes.TrimSpace(ev.Data), []byte("[DONE]")) {
		b.out = b.flushAll(b.out)
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	data, keep, err := b.rewrite(ev.Data)
	if err != nil {
		if b.held() > 0 {
			b.finish(fmt.Errorf("stream restore: %w", err))
			return
		}
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	if !keep {
		b.dropped++
		return
	}
	ev.Data = data
	b.out = sse.AppendEvent(b.out, ev)
}

func (b *StreamBody) held() int {
	n := 0
	for i := range b.channels {
		n += b.channels[i].stream.Held()
	}
	return n
}

func (b *StreamBody) known(k channelKey) bool {
	for i := range b.channels {
		if b.channels[i].key == k {
			return true
		}
	}
	return false
}

func (b *StreamBody) reserve(choices []sChoice) error {
	need := 0
	for i := range choices {
		ch := &choices[i]
		if ch.Delta == nil {
			continue
		}
		keys := make([]channelKey, 0, 2+len(ch.Delta.ToolCalls))
		keys = append(keys, channelKey{ch.Index, kindContent, 0}, channelKey{ch.Index, kindRefusal, 0})
		for _, tc := range ch.Delta.ToolCalls {
			keys = append(keys, channelKey{ch.Index, kindTool, tc.Index})
		}
		for _, k := range keys {
			if !b.known(k) {
				need++
			}
		}
	}
	if len(b.channels)+need > codec.MaxChannels {
		return codec.ErrTooManyChannels
	}
	return nil
}

type payloadProbe struct {
	Usage jsontext.Value `json:"usage"`
}

func hasNoPayload(extra jsontext.Value) bool {
	if len(extra) == 0 {
		return true
	}
	var p payloadProbe
	if err := json.Unmarshal(extra, &p); err != nil {
		return false
	}
	return len(p.Usage) == 0 || string(p.Usage) == "null"
}

func (b *StreamBody) channel(k channelKey) (*restore.Stream, error) {
	for i := range b.channels {
		if b.channels[i].key == k {
			return &b.channels[i].stream, nil
		}
	}
	if len(b.channels) >= codec.MaxChannels {
		return nil, codec.ErrTooManyChannels
	}
	o := b.opts
	if k.kind == kindTool {
		o.Encoding = restore.JSONString
		o.Tool = true
	} else {
		o.Encoding = restore.Raw
	}
	b.channels = append(b.channels, channel{key: k, stream: restore.NewStream(b.set, o)})
	return &b.channels[len(b.channels)-1].stream, nil
}

func (b *StreamBody) feed(k channelKey, s *string) error {
	if s == nil {
		return nil
	}
	st, err := b.channel(k)
	if err != nil {
		return err
	}
	b.scratch = st.Feed(b.scratch[:0], []byte(*s))
	if string(b.scratch) != *s {
		*s = string(b.scratch)
		b.changed = true
	}
	return nil
}

func (b *StreamBody) rewrite(data []byte) (encoded []byte, keep bool, err error) {
	var c sChunk
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false, fmt.Errorf("decode chunk: %w", err)
	}
	if len(c.Choices) == 0 {
		return data, true, nil
	}
	b.header = append(b.header[:0], c.Extra...)
	if err := b.reserve(c.Choices); err != nil {
		return nil, false, err
	}
	b.changed = false
	empty := true
	for i := range c.Choices {
		ch := &c.Choices[i]
		if ch.Delta == nil {
			ch.Delta = &sDelta{}
		}
		if err := b.feedChoice(ch); err != nil {
			return nil, false, err
		}
		if ch.FinishReason != nil {
			b.flushChoice(ch)
		}
		if !choiceEmpty(ch) {
			empty = false
		}
	}
	if empty && hasNoPayload(c.Extra) {
		return nil, false, nil
	}
	if !b.changed {
		return data, true, nil
	}
	out, err := json.Marshal(&c)
	if err != nil {
		return nil, false, fmt.Errorf("encode chunk: %w", err)
	}
	return out, true, nil
}

func (b *StreamBody) feedChoice(ch *sChoice) error {
	d := ch.Delta
	if err := b.feed(channelKey{ch.Index, kindContent, 0}, d.Content); err != nil {
		return err
	}
	if err := b.feed(channelKey{ch.Index, kindRefusal, 0}, d.Refusal); err != nil {
		return err
	}
	for j := range d.ToolCalls {
		tc := &d.ToolCalls[j]
		if tc.Function == nil {
			continue
		}
		if err := b.feed(channelKey{ch.Index, kindTool, tc.Index}, tc.Function.Arguments); err != nil {
			return err
		}
	}
	return nil
}

func appendFlushed(s *string, extra []byte) *string {
	if len(extra) == 0 {
		return s
	}
	v := string(extra)
	if s != nil {
		v = *s + v
	}
	return &v
}

func (b *StreamBody) flushChoice(ch *sChoice) {
	d := ch.Delta
	for i := range b.channels {
		c := &b.channels[i]
		if c.key.choice != ch.Index || c.stream.Held() == 0 {
			continue
		}
		b.scratch = c.stream.Flush(b.scratch[:0])
		b.changed = true
		switch c.key.kind {
		case kindContent:
			d.Content = appendFlushed(d.Content, b.scratch)
		case kindRefusal:
			d.Refusal = appendFlushed(d.Refusal, b.scratch)
		default:
			b.appendToolArgs(d, c.key.tool, b.scratch)
		}
	}
}

func (b *StreamBody) appendToolArgs(d *sDelta, tool int, extra []byte) {
	for j := range d.ToolCalls {
		tc := &d.ToolCalls[j]
		if tc.Index == tool {
			if tc.Function == nil {
				tc.Function = &sFunction{}
			}
			tc.Function.Arguments = appendFlushed(tc.Function.Arguments, extra)
			return
		}
	}
	d.ToolCalls = append(d.ToolCalls, sTool{Index: tool, Function: &sFunction{Arguments: appendFlushed(nil, extra)}})
}

func emptyObject(v jsontext.Value) bool {
	t := bytes.TrimSpace(v)
	return len(t) == 0 || string(t) == "{}"
}

func choiceEmpty(ch *sChoice) bool {
	if ch.FinishReason != nil || !emptyObject(ch.Extra) {
		return false
	}
	d := ch.Delta
	if !emptyObject(d.Extra) || (d.Content != nil && *d.Content != "") || (d.Refusal != nil && *d.Refusal != "") {
		return false
	}
	for _, tc := range d.ToolCalls {
		if !emptyObject(tc.Extra) {
			return false
		}
		if tc.Function != nil &&
			(!emptyObject(tc.Function.Extra) || (tc.Function.Arguments != nil && *tc.Function.Arguments != "")) {
			return false
		}
	}
	return true
}

func (b *StreamBody) flushAll(dst []byte) []byte {
	if b.held() == 0 {
		return dst
	}
	var c sChunk
	c.Extra = jsontext.Value(`{"object":"chat.completion.chunk"}`)
	if len(b.header) > 0 {
		c.Extra = withoutUsage(b.header)
	}
	for i := range b.channels {
		ch := &b.channels[i]
		if ch.stream.Held() == 0 {
			continue
		}
		idx := -1
		for k := range c.Choices {
			if c.Choices[k].Index == ch.key.choice {
				idx = k
			}
		}
		if idx < 0 {
			c.Choices = append(c.Choices, sChoice{Index: ch.key.choice, Delta: &sDelta{}})
			idx = len(c.Choices) - 1
		}
		b.flushChoice(&c.Choices[idx])
	}
	out, err := json.Marshal(&c)
	if err != nil {
		return dst
	}
	return sse.AppendEvent(dst, sse.Event{Data: out, HasData: true})
}

func withoutUsage(extra jsontext.Value) jsontext.Value {
	var m map[string]jsontext.Value
	if err := json.Unmarshal(extra, &m); err != nil {
		return jsontext.Value(`{"object":"chat.completion.chunk"}`)
	}
	delete(m, "usage")
	out, err := json.Marshal(m, json.Deterministic(true))
	if err != nil {
		return jsontext.Value(`{"object":"chat.completion.chunk"}`)
	}
	return out
}
