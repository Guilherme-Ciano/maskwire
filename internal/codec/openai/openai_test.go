package openai

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func session(tb testing.TB) *mask.Session {
	tb.Helper()
	kr, err := token.NewKeyring(bytes.Repeat([]byte{1}, 32), 1)
	if err != nil {
		tb.Fatal(err)
	}
	tk, err := kr.Tenant("acme")
	if err != nil {
		tb.Fatal(err)
	}
	eng := mask.NewEngine(detect.New(detect.Options{}), 0)
	return eng.NewSession(&mask.Tenant{ID: "acme", Key: tk, Policy: policy.New(policy.Enforce, policy.Mask)})
}

const reqBody = `{"model":"gpt-4.1-mini","stream":true,"messages":[` +
	`{"role":"system","content":"You are helpful."},` +
	`{"role":"user","content":"Email ana.silva@empresa.com.br about CPF 529.982.247-25"},` +
	`{"role":"user","content":[{"type":"text","text":"IBAN PT50 0002 0123 1234 5678 9015 4"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD 529.982.247-25"}}]},` +
	`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"send","arguments":"{\"to\":\"ana.silva@empresa.com.br\",\"n\":52998224725}"}}]}],` +
	`"tools":[{"type":"function","function":{"name":"send","description":"send to ana.silva@empresa.com.br"}}],"user":"ana.silva@empresa.com.br"}`

func TestMaskRequest(t *testing.T) {
	t.Parallel()
	s := session(t)
	out, err := New().MaskRequest(nil, []byte(reqBody), s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Count(got, "ana.silva@empresa.com.br") != 1 ||
		!strings.Contains(got, `"description":"send to ana.silva@empresa.com.br"`) {
		t.Fatalf("expected only the tool description to keep the email: %s", got)
	}
	if strings.Contains(got, "PT50") || !strings.Contains(got, "base64,QUJD 529.982.247-25") ||
		strings.Count(got, "529.982.247-25") != 1 {
		t.Fatalf("unexpected masking: %s", got)
	}
	if !strings.Contains(got, `\"n\":52998224725`) {
		t.Fatalf("numbers inside arguments must stay: %s", got)
	}
	if !jsontext.Value(out).IsValid() {
		t.Fatalf("invalid json: %s", got)
	}
	again, err := New().MaskRequest(nil, []byte(reqBody), session(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, again) {
		t.Fatal("masking must be deterministic")
	}
	meta, err := ParseMeta(out)
	if err != nil || !meta.Stream || meta.Model != "gpt-4.1-mini" {
		t.Fatalf("meta %+v %v", meta, err)
	}
}

func maskedText(t *testing.T, s *mask.Session, text string) string {
	t.Helper()
	out, err := s.MaskText(nil, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRestoreBody(t *testing.T) {
	t.Parallel()
	s := session(t)
	tok := maskedText(t, s, "ana.silva@empresa.com.br")
	body := `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"Hi ` + tok + `","tool_calls":[{"id":"c","type":"function","function":{"name":"send","arguments":"{\"to\":\"` + tok + `\"}"}}]},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`
	out, st, err := New().RestoreBody(nil, []byte(body), s, restore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Restored != 2 || !strings.Contains(string(out), `"content":"Hi ana.silva@empresa.com.br"`) ||
		!strings.Contains(string(out), `{\"to\":\"ana.silva@empresa.com.br\"}`) {
		t.Fatalf("got %s (%+v)", out, st)
	}
}

func sseFrom(text, toolArgs string, rng *rand.Rand) string {
	var sb strings.Builder
	sb.WriteString(
		`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}` + "\n\n",
	)
	for i := 0; i < len(text); {
		n := min(1+rng.IntN(7), len(text)-i)
		piece := jsonx.Quote(nil, []byte(text[i:i+n]))
		sb.WriteString(
			`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":` + string(
				piece,
			) + `},"finish_reason":null}]}` + "\n\n",
		)
		i += n
	}
	for i := 0; i < len(toolArgs); {
		n := min(1+rng.IntN(7), len(toolArgs)-i)
		piece := jsonx.Quote(nil, []byte(toolArgs[i:i+n]))
		sb.WriteString(
			`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":` + string(
				piece,
			) + `}}]},"finish_reason":null}]}` + "\n\n",
		)
		i += n
	}
	sb.WriteString(
		`data: {"id":"c","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n: keepalive\n\ndata: [DONE]\n\n",
	)
	return sb.String()
}

func collect(t *testing.T, r io.Reader) (contentText, argsText string) {
	t.Helper()
	rd := bytes.NewBuffer(nil)
	if _, err := io.Copy(rd, r); err != nil {
		t.Fatal(err)
	}
	var content, args strings.Builder
	for line := range strings.SplitSeq(rd.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var c sChunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			t.Fatalf("bad chunk %q: %v", data, err)
		}
		for _, ch := range c.Choices {
			if ch.Delta != nil && ch.Delta.Content != nil {
				content.WriteString(*ch.Delta.Content)
			}
			if ch.Delta != nil {
				for _, tc := range ch.Delta.ToolCalls {
					if tc.Function != nil && tc.Function.Arguments != nil {
						args.WriteString(*tc.Function.Arguments)
					}
				}
			}
		}
	}
	return content.String(), args.String()
}

func TestStreamRestoreRandomChunking(t *testing.T) {
	t.Parallel()
	original := `Dear ana.silva@empresa.com.br, CPF 529.982.247-25 and IBAN PT50 0002 0123 1234 5678 9015 4 are on file. "quoted" \ ok`
	for seed := range uint64(200) {
		s := session(t)
		masked := maskedText(t, s, original)
		args := `{"to":"` + maskedText(t, s, `obrien@x.pt`) + `"}`
		rng := rand.New(rand.NewPCG(seed, seed))
		body := NewStreamBody(
			io.NopCloser(strings.NewReader(sseFrom("Echo: "+masked, args, rng))),
			s,
			codec.StreamOptions{},
		)
		content, gotArgs := collect(t, body)
		if content != "Echo: "+original {
			t.Fatalf("seed %d: content %q", seed, content)
		}
		if gotArgs != `{"to":"obrien@x.pt"}` {
			t.Fatalf("seed %d: args %q", seed, gotArgs)
		}
		if st := body.Stats(); st.Restored != 4 || st.Foreign != 0 {
			t.Fatalf("seed %d: stats %+v", seed, st)
		}
	}
}

func TestStreamFlushAtEOFWithoutDone(t *testing.T) {
	t.Parallel()
	s := session(t)
	in := `data: {"choices":[{"index":0,"delta":{"content":"tail [EMAIL_"},"finish_reason":null}]}` + "\n\n"
	content, _ := collect(t, NewStreamBody(io.NopCloser(strings.NewReader(in)), s, codec.StreamOptions{}))
	if content != "tail [EMAIL_" {
		t.Fatalf("got %q", content)
	}
}

func TestSynthesize(t *testing.T) {
	t.Parallel()
	body := `{"id":"x","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"total_tokens":2}}`
	out, err := Synthesize(nil, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := collect(t, bytes.NewReader(out))
	if content != "hello" || !strings.HasSuffix(string(out), "data: [DONE]\n\n") ||
		!strings.Contains(string(out), `"finish_reason":"stop"`) {
		t.Fatalf("got %s", out)
	}
}

func FuzzMaskRequest(f *testing.F) {
	f.Add([]byte(reqBody))
	f.Add([]byte(`{"messages":[{"content":[{"text":"x"}]}]}`))
	c := New()
	s := session(f)
	f.Fuzz(func(t *testing.T, body []byte) {
		out, err := c.MaskRequest(nil, body, s)
		if err != nil {
			return
		}
		if !jsontext.Value(out).IsValid() {
			t.Fatalf("invalid output for %q: %q", body, out)
		}
	})
}
