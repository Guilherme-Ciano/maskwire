package openai

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

func dropNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e == nil {
				delete(x, k)
				continue
			}
			x[k] = dropNulls(e)
		}
	case []any:
		for i, e := range x {
			x[i] = dropNulls(e)
		}
	}
	return v
}

func normalised(t *testing.T, stream []byte) []any {
	t.Helper()
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
		out = append(out, dropNulls(v))
	}
	return out
}

func randomStream(rng *rand.Rand, tokens []string) []byte {
	pieces := append([]string{"Hello ", "CPF ", "ok", "", " and ", "é", "\"q\"", "\\", "<b>", "\n"}, tokens...)
	var text strings.Builder
	for range 1 + rng.IntN(12) {
		text.WriteString(pieces[rng.IntN(len(pieces))])
	}
	full := text.String()
	var src strings.Builder
	head := `"id":"c1","object":"chat.completion.chunk","created":1,"model":"m"`
	for i := 0; i < len(full); {
		n := 1 + rng.IntN(6)
		part := full[i:min(i+n, len(full))]
		i += n
		q := string(jsonx.Quote(nil, []byte(part)))
		var delta string
		switch rng.IntN(6) {
		case 0:
			delta = `{"role":"assistant","content":` + q + `}`
		case 1:
			delta = `{"content":` + q + `,"refusal":null}`
		case 2:
			delta = `{"refusal":` + q + `}`
		default:
			delta = `{"content":` + q + `}`
		}
		choice := `{"index":0,"delta":` + delta + `,"finish_reason":null}`
		if rng.IntN(2) == 0 {
			choice = `{"index":0,"delta":` + delta + `,"logprobs":null,"finish_reason":null}`
		}
		choices := choice
		if rng.IntN(5) == 0 {
			choices += `,{"index":1,"delta":{"content":` + q + `},"finish_reason":null}`
		}
		extra := ""
		if rng.IntN(8) == 0 {
			extra = `,"usage":null`
		}
		if rng.IntN(2) == 0 {
			fmt.Fprintf(&src, "data: {%s,\"choices\":[%s]%s}\n\n", head, choices, extra)
		} else {
			fmt.Fprintf(&src, "data: {\"choices\":[%s],%s%s}\n\n", choices, head, extra)
		}
	}
	src.WriteString(`data: {` + head + `,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n")
	src.WriteString("data: [DONE]\n\n")
	return []byte(src.String())
}

func TestFastChunksMatchStructPath(t *testing.T) {
	t.Parallel()
	s := session(t)
	tokens := make([]string, 0, 2)
	for _, v := range []string{"ana.souza@correio.pt", "CPF 529.982.247-25"} {
		masked, err := s.MaskText(nil, []byte(v))
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, string(masked))
	}
	rng := rand.New(rand.NewPCG(5, 8))
	for range 3000 {
		src := randomStream(rng, tokens)
		fast := NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{})
		slow := NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{})
		slow.parser = nil
		got, err1 := io.ReadAll(fast)
		want, err2 := io.ReadAll(slow)
		if err1 != nil || err2 != nil {
			t.Fatalf("errors: %v %v", err1, err2)
		}
		if g, w := normalised(t, got), normalised(t, want); !reflect.DeepEqual(g, w) {
			for k := range max(len(g), len(w)) {
				var a, b any
				if k < len(g) {
					a = g[k]
				}
				if k < len(w) {
					b = w[k]
				}
				if !reflect.DeepEqual(a, b) {
					t.Fatalf("event %d differs\nfast:   %v\nstruct: %v", k, a, b)
				}
			}
		}
		if fast.Stats() != slow.Stats() {
			t.Fatalf("stats differ: %+v vs %+v", fast.Stats(), slow.Stats())
		}
	}
}
