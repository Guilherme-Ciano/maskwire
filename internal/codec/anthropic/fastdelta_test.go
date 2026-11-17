package anthropic

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

func normalised(stream []byte) []any {
	var out []any
	for line := range strings.SplitSeq(string(stream), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			if line != "" {
				out = append(out, line)
			}
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			out = append(out, data)
			continue
		}
		out = append(out, v)
	}
	return out
}

func anthropicStream(rng *rand.Rand, tokens []string) []byte {
	pieces := append([]string{"Hi ", "CPF ", "", " and ", "é", "\"q\"", "\\", "\n", "{\"a\":"}, tokens...)
	var src strings.Builder
	src.WriteString("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")
	blocks := 1 + rng.IntN(3)
	for idx := range blocks {
		kind, dtype, field := "text", "text_delta", "text"
		switch rng.IntN(3) {
		case 1:
			kind, dtype, field = "tool_use", "input_json_delta", "partial_json"
		case 2:
			kind, dtype, field = "thinking", "thinking_delta", "thinking"
		}
		fmt.Fprintf(
			&src,
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":%d,\"content_block\":{\"type\":%q}}\n\n",
			idx,
			kind,
		)
		var text strings.Builder
		for range 1 + rng.IntN(8) {
			text.WriteString(pieces[rng.IntN(len(pieces))])
		}
		full := text.String()
		for i := 0; i < len(full); {
			n := 1 + rng.IntN(5)
			part := jsonx.Quote(nil, []byte(full[i:min(i+n, len(full))]))
			i += n
			if rng.IntN(2) == 0 {
				fmt.Fprintf(
					&src,
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":%q,%q:%s}}\n\n",
					idx,
					dtype,
					field,
					part,
				)
			} else {
				fmt.Fprintf(
					&src,
					"event: content_block_delta\ndata: {\"delta\":{%q:%s,\"type\":%q},\"index\":%d,\"type\":\"content_block_delta\"}\n\n",
					field,
					part,
					dtype,
					idx,
				)
			}
		}
		if rng.IntN(4) > 0 {
			fmt.Fprintf(
				&src,
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n",
				idx,
			)
		}
	}
	src.WriteString("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	return []byte(src.String())
}

func TestFastDeltasMatchStructPath(t *testing.T) {
	t.Parallel()
	s := session(t)
	tokens := []string{masked(t, s, "ana.souza@correio.pt"), masked(t, s, "CPF 529.982.247-25")}
	rng := rand.New(rand.NewPCG(13, 21))
	for range 3000 {
		src := anthropicStream(rng, tokens)
		fast := NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{})
		slow := NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{})
		slow.parser = nil
		got, err1 := io.ReadAll(fast)
		want, err2 := io.ReadAll(slow)
		if err1 != nil || err2 != nil {
			t.Fatalf("errors: %v %v", err1, err2)
		}
		if g, w := normalised(got), normalised(want); !reflect.DeepEqual(g, w) {
			t.Fatalf("source:\n%s\nfast:\n%s\nstruct:\n%s", src, got, want)
		}
		if fast.Stats() != slow.Stats() {
			t.Fatalf("stats differ: %+v vs %+v", fast.Stats(), slow.Stats())
		}
	}
}

func BenchmarkStreamEvents(b *testing.B) {
	s := session(b)
	text := strings.Repeat(
		"Reply to "+masked(b, s, "ana.souza@correio.pt")+" about CPF "+masked(b, s, "CPF 529.982.247-25")+". ",
		60,
	)
	var src strings.Builder
	src.WriteString(
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
	)
	events := 0
	for i := 0; i < len(text); i += 4 {
		part := jsonx.Quote(nil, []byte(text[i:min(i+4, len(text))]))
		fmt.Fprintf(
			&src,
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n",
			part,
		)
		events++
	}
	src.WriteString("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	data := []byte(src.String())
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ReportMetric(float64(events), "events/op")
	for b.Loop() {
		body := NewStreamBody(io.NopCloser(bytes.NewReader(data)), s, codec.StreamOptions{})
		if _, err := io.Copy(io.Discard, body); err != nil {
			b.Fatal(err)
		}
	}
}
