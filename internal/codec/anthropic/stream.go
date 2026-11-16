package anthropic

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/sse"
)

type blockKind uint8

const (
	kindPassthrough blockKind = iota
	kindText
	kindJSON
)

type envelope struct {
	Type         string         `json:"type"`
	Index        int            `json:"index"`
	ContentBlock *startBlock    `json:"content_block,omitzero"`
	Delta        *delta         `json:"delta,omitzero"`
	Extra        jsontext.Value `json:",embed"`
}

type startBlock struct {
	Type  string         `json:"type"`
	Extra jsontext.Value `json:",embed"`
}

type delta struct {
	Type        string         `json:"type"`
	Text        *string        `json:"text,omitzero"`
	PartialJSON *string        `json:"partial_json,omitzero"`
	Extra       jsontext.Value `json:",embed"`
}

type channel struct {
	index  int
	kind   blockKind
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
	events   int
	dropped  int
	onClose  func(restore.Stats, int, int)
	retired  restore.Stats
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
	s := b.retired
	for i := range b.channels {
		s.Add(b.channels[i].stream.Stats())
	}
	return s
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
	b.done, b.err = true, err
	b.idle.Stop()
}

func (b *StreamBody) find(index int) *channel {
	for i := range b.channels {
		if b.channels[i].index == index {
			return &b.channels[i]
		}
	}
	return nil
}

func (b *StreamBody) step() {
	ev, err := b.rd.Next()
	if errors.Is(err, io.EOF) {
		for i := range b.channels {
			b.out = b.flush(b.out, &b.channels[i])
		}
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
	var env envelope
	if err := json.Unmarshal(ev.Data, &env); err != nil {
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	forward, err := b.dispatch(&ev, &env)
	if err != nil {
		b.finish(err)
		return
	}
	if !forward {
		b.dropped++
		return
	}
	b.out = sse.AppendEvent(b.out, ev)
}

func (b *StreamBody) dispatch(ev *sse.Event, env *envelope) (forward bool, err error) {
	switch env.Type {
	case evBlockStart:
		return true, b.start(env)
	case evBlockDelta:
		data, keep, err := b.delta(env)
		if err != nil || !keep {
			return false, err
		}
		if data != nil {
			ev.Data = data
		}
	case evBlockStop:
		if c := b.find(env.Index); c != nil {
			b.out = b.flush(b.out, c)
		}
	case evMessageStop:
		for i := range b.channels {
			b.out = b.flush(b.out, &b.channels[i])
		}
	}
	return true, nil
}

func (b *StreamBody) start(env *envelope) error {
	kind := kindPassthrough
	if env.ContentBlock != nil {
		switch env.ContentBlock.Type {
		case blockTypeText:
			kind = kindText
		case blockTypeTool, "server_tool_use":
			kind = kindJSON
		}
	}
	o := b.opts
	o.Encoding = restore.Raw
	if kind == kindJSON {
		o.Encoding = restore.JSONString
	}
	if c := b.find(env.Index); c != nil {
		b.out = b.flush(b.out, c)
		b.retired.Add(c.stream.Stats())
		c.kind, c.stream = kind, restore.NewStream(b.set, o)
		return nil
	}
	if len(b.channels) >= codec.MaxChannels {
		return codec.ErrTooManyChannels
	}
	b.channels = append(b.channels, channel{index: env.Index, kind: kind, stream: restore.NewStream(b.set, o)})
	return nil
}

func (b *StreamBody) delta(env *envelope) (data []byte, keep bool, err error) {
	c := b.find(env.Index)
	if c == nil || c.kind == kindPassthrough || env.Delta == nil {
		return nil, true, nil
	}
	var field *string
	switch {
	case c.kind == kindText && env.Delta.Type == deltaText:
		field = env.Delta.Text
	case c.kind == kindJSON && env.Delta.Type == deltaJSON:
		field = env.Delta.PartialJSON
	}
	if field == nil {
		b.out = b.flush(b.out, c)
		return nil, true, nil
	}
	b.scratch = c.stream.Feed(b.scratch[:0], []byte(*field))
	if len(b.scratch) == 0 {
		return nil, false, nil
	}
	if string(b.scratch) == *field {
		return nil, true, nil
	}
	*field = string(b.scratch)
	out, err := json.Marshal(env)
	if err != nil {
		return nil, false, fmt.Errorf("encode anthropic delta: %w", err)
	}
	return out, true, nil
}

func (b *StreamBody) flush(dst []byte, c *channel) []byte {
	if c.stream.Held() == 0 {
		return dst
	}
	b.scratch = c.stream.Flush(b.scratch[:0])
	text := string(b.scratch)
	d := &delta{Type: deltaText, Text: &text}
	if c.kind == kindJSON {
		d = &delta{Type: deltaJSON, PartialJSON: &text}
	}
	out, err := json.Marshal(&envelope{Type: evBlockDelta, Index: c.index, Delta: d})
	if err != nil {
		return dst
	}
	return sse.AppendEvent(dst, sse.Event{Name: []byte(evBlockDelta), Data: out, HasData: true})
}
