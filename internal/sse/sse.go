package sse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

const DefaultMaxEvent = 1 << 20

var ErrEventTooLarge = errors.New("sse event exceeds size limit")

type Event struct {
	Name    []byte
	Data    []byte
	ID      []byte
	Retry   []byte
	HasData bool
	HasID   bool
	Comment bool
}

type Reader struct {
	sc       *bufio.Scanner
	max      int
	name     []byte
	data     []byte
	id       []byte
	retry    []byte
	comment  []byte
	hasData  bool
	hasID    bool
	hasOther bool
}

func NewReader(r io.Reader, maxEvent int) *Reader {
	if maxEvent <= 0 {
		maxEvent = DefaultMaxEvent
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxEvent)
	return &Reader{sc: sc, max: maxEvent}
}

func (r *Reader) reset() {
	r.name = r.name[:0]
	r.data = r.data[:0]
	r.id = r.id[:0]
	r.retry = r.retry[:0]
	r.comment = r.comment[:0]
	r.hasData, r.hasID, r.hasOther = false, false, false
}

func (r *Reader) event() Event {
	if !r.hasData && !r.hasID && !r.hasOther && len(r.name) == 0 {
		return Event{Comment: true, Data: r.comment}
	}
	return Event{Name: r.name, Data: r.data, ID: r.id, Retry: r.retry, HasData: r.hasData, HasID: r.hasID}
}

func (r *Reader) pending() bool {
	return r.hasData || r.hasID || r.hasOther || len(r.name) > 0 || len(r.comment) > 0
}

func (r *Reader) Next() (Event, error) {
	r.reset()
	for r.sc.Scan() {
		line := bytes.TrimSuffix(r.sc.Bytes(), []byte{'\r'})
		if len(line) == 0 {
			if r.pending() {
				return r.event(), nil
			}
			continue
		}
		if err := r.field(line); err != nil {
			return Event{}, err
		}
	}
	if err := r.sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return Event{}, ErrEventTooLarge
		}
		return Event{}, fmt.Errorf("read sse: %w", err)
	}
	if r.pending() {
		return r.event(), nil
	}
	return Event{}, io.EOF
}

func (r *Reader) field(line []byte) error {
	if line[0] == ':' {
		if len(r.comment) > 0 {
			r.comment = append(r.comment, '\n')
		}
		r.comment = append(r.comment, line[1:]...)
		return nil
	}
	name, value, found := bytes.Cut(line, []byte{':'})
	if found && len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	switch string(name) {
	case "data":
		if r.hasData {
			r.data = append(r.data, '\n')
		}
		r.data = append(r.data, value...)
		r.hasData = true
	case "event":
		r.name = append(r.name[:0], value...)
	case "id":
		r.id = append(r.id[:0], value...)
		r.hasID = true
	case "retry":
		r.retry = append(r.retry[:0], value...)
		r.hasOther = true
	default:
		r.hasOther = true
	}
	if len(r.data)+len(r.name)+len(r.comment) > r.max {
		return ErrEventTooLarge
	}
	return nil
}

func AppendEvent(dst []byte, e Event) []byte {
	if e.Comment {
		for line := range bytes.SplitSeq(e.Data, []byte{'\n'}) {
			dst = append(dst, ':')
			dst = append(dst, line...)
			dst = append(dst, '\n')
		}
		return append(dst, '\n')
	}
	if len(e.Name) > 0 {
		dst = append(dst, "event: "...)
		dst = append(dst, e.Name...)
		dst = append(dst, '\n')
	}
	if e.HasID {
		dst = append(dst, "id: "...)
		dst = append(dst, e.ID...)
		dst = append(dst, '\n')
	}
	if len(e.Retry) > 0 {
		dst = append(dst, "retry: "...)
		dst = append(dst, e.Retry...)
		dst = append(dst, '\n')
	}
	if e.HasData {
		for line := range bytes.SplitSeq(e.Data, []byte{'\n'}) {
			dst = append(dst, "data: "...)
			dst = append(dst, line...)
			dst = append(dst, '\n')
		}
	}
	return append(dst, '\n')
}
