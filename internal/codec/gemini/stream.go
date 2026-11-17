package gemini

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/sse"
)

type channel struct {
	candidate int
	stream    restore.Stream
}

type ending struct {
	candidate int
	lastText  int
}

type StreamBody struct {
	src      io.ReadCloser
	rd       *sse.Reader
	set      restore.Set
	opts     restore.Options
	rw       *jsonx.Rewriter
	channels []channel
	endings  []ending
	out      []byte
	pos      int
	plain    []byte
	restored []byte
	done     bool
	err      error
	idle     *codec.IdleReader
	events   int
	onClose  func(restore.Stats, int, int)
	extra    restore.Stats
}

func NewStreamBody(src io.ReadCloser, set restore.Set, o codec.StreamOptions) *StreamBody {
	idle := codec.WatchIdle(src, o.Idle, o.OnStall)
	return &StreamBody{
		src:     src,
		rd:      sse.NewReader(idle, o.MaxEvent),
		set:     set,
		opts:    o.Restore,
		rw:      jsonx.NewRewriter(),
		idle:    idle,
		onClose: o.OnClose,
	}
}

func (b *StreamBody) Stats() restore.Stats {
	s := b.extra
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
		b.onClose(b.Stats(), b.events, 0)
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

func (b *StreamBody) step() {
	ev, err := b.rd.Next()
	if errors.Is(err, io.EOF) {
		for i := range b.channels {
			b.out = b.flushEvent(b.out, &b.channels[i])
		}
		b.finish(io.EOF)
		return
	}
	if err != nil {
		b.finish(fmt.Errorf("stream restore: %w", err))
		return
	}
	b.events++
	if ev.Comment || !ev.HasData || !jsonx.Valid(ev.Data) {
		b.out = sse.AppendEvent(b.out, ev)
		return
	}
	data, err := b.rewrite(ev.Data)
	if err != nil {
		b.finish(fmt.Errorf("stream restore: %w", err))
		return
	}
	ev.Data = data
	b.out = sse.AppendEvent(b.out, ev)
}

func (b *StreamBody) channel(candidate int) (*restore.Stream, error) {
	for i := range b.channels {
		if b.channels[i].candidate == candidate {
			return &b.channels[i].stream, nil
		}
	}
	if len(b.channels) >= codec.MaxChannels {
		return nil, codec.ErrTooManyChannels
	}
	o := b.opts
	o.Encoding = restore.Raw
	b.channels = append(b.channels, channel{candidate: candidate, stream: restore.NewStream(b.set, o)})
	return &b.channels[len(b.channels)-1].stream, nil
}

type finishProbe struct {
	Candidates []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []map[string]jsontext.Value `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (b *StreamBody) findEndings(data []byte) {
	b.endings = b.endings[:0]
	if !bytes.Contains(data, []byte(`"finishReason"`)) {
		return
	}
	var p finishProbe
	if json.Unmarshal(data, &p) != nil {
		return
	}
	for i, c := range p.Candidates {
		if c.FinishReason == "" {
			continue
		}
		last := -1
		for j, part := range c.Content.Parts {
			if _, ok := part["text"]; ok {
				last = j
			}
		}
		b.endings = append(b.endings, ending{candidate: i, lastText: last})
	}
}

func (b *StreamBody) ends(candidate, part int) bool {
	for _, e := range b.endings {
		if e.candidate == candidate && e.lastText == part {
			return true
		}
	}
	return false
}

func (b *StreamBody) rewrite(data []byte) ([]byte, error) {
	b.findEndings(data)
	var pre []byte
	for _, e := range b.endings {
		if e.lastText >= 0 {
			continue
		}
		if st, err := b.channel(e.candidate); err == nil && st.Held() > 0 {
			pre = b.flushEvent(pre, &b.channels[b.index(e.candidate)])
		}
	}
	out, err := b.rw.Rewrite(nil, data, func(dst []byte, path []jsonx.Frame, raw []byte) ([]byte, bool, error) {
		switch {
		case !jsonx.IsString(raw):
			return dst, false, nil
		case partText(path):
			return b.feedText(dst, path[1].Index, path[4].Index, raw)
		case callArgs(path):
			return b.restoreArg(dst, raw)
		}
		return dst, false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("rewrite gemini chunk: %w", err)
	}
	if len(pre) > 0 {
		b.out = append(b.out, pre...)
	}
	return out, nil
}

func (b *StreamBody) index(candidate int) int {
	for i := range b.channels {
		if b.channels[i].candidate == candidate {
			return i
		}
	}
	return -1
}

func (b *StreamBody) feedText(dst []byte, candidate, part int, raw []byte) (out []byte, changed bool, err error) {
	st, cerr := b.channel(candidate)
	if cerr != nil {
		return dst, false, cerr
	}
	b.plain, err = jsonx.Unquote(b.plain[:0], raw)
	if err != nil {
		return dst, false, err
	}
	b.restored = st.Feed(b.restored[:0], b.plain)
	if b.ends(candidate, part) {
		b.restored = st.Flush(b.restored)
	}
	if bytes.Equal(b.restored, b.plain) {
		return dst, false, nil
	}
	return jsonx.Quote(dst, b.restored), true, nil
}

func (b *StreamBody) restoreArg(dst, raw []byte) (out []byte, changed bool, err error) {
	b.plain, err = jsonx.Unquote(b.plain[:0], raw)
	if err != nil {
		return dst, false, err
	}
	o := b.opts
	o.Encoding = restore.Raw
	o.Tool = true
	var st restore.Stats
	b.restored, st = restore.Restore(b.restored[:0], b.plain, b.set, o)
	b.extra.Add(&st)
	if st.Restored == 0 && st.Foreign == 0 {
		return dst, false, nil
	}
	return jsonx.Quote(dst, b.restored), true, nil
}

func (b *StreamBody) flushEvent(dst []byte, c *channel) []byte {
	if c.stream.Held() == 0 {
		return dst
	}
	b.restored = c.stream.Flush(b.restored[:0])
	data := []byte(`{"candidates":[{"content":{"parts":[{"text":`)
	data = jsonx.Quote(data, b.restored)
	data = append(data, `}],"role":"model"},"index":`...)
	data = strconv.AppendInt(data, int64(c.candidate), 10)
	data = append(data, "}]}"...)
	return sse.AppendEvent(dst, sse.Event{Data: data, HasData: true})
}
