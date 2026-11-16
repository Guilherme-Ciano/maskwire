package anthropic

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"io"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func session(t *testing.T) *mask.Session {
	t.Helper()
	kr, err := token.NewKeyring(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := kr.Tenant("acme")
	if err != nil {
		t.Fatal(err)
	}
	return mask.NewEngine(detect.New(detect.Options{}), 0).NewSession(
		&mask.Tenant{ID: "acme", Key: tk, Policy: policy.New(policy.Enforce, policy.Mask)})
}

var leaks = []string{"529.982.247-25", "52998224725", "ana.souza@correio.pt"}

func TestMaskRequestCoversShapes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"system string":  `{"system":"contact ana.souza@correio.pt","messages":[]}`,
		"system blocks":  `{"system":[{"type":"text","text":"contact ana.souza@correio.pt"}],"messages":[]}`,
		"escaped key":    `{"messages":[{"role":"user","content":"cpf 529.982.247-25"}]}`,
		"tool input num": `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"x","input":{"cpf":52998224725}}]}]}`,
		"tool result":    `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":[{"type":"text","text":"ana.souza@correio.pt"}]}]}]}`,
		"text document": `{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"text",` +
			`"media_type":"text/plain","data":"cpf 529.982.247-25"},"title":"ana.souza@correio.pt"}]}]}`,
		"metadata":         `{"messages":[],"metadata":{"user_id":"ana.souza@correio.pt"}}`,
		"tool description": `{"messages":[],"tools":[{"name":"x","description":"mail ana.souza@correio.pt","input_schema":{"type":"object"}}]}`,
	}
	c := New()
	for name, body := range cases {
		out, err := c.MaskRequest(nil, []byte(body), session(t))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, leak := range leaks {
			if strings.Contains(string(out), leak) {
				t.Errorf("%s: %q leaked in %s", name, leak, out)
			}
		}
	}
}

func TestMaskRequestLeavesProtectedFields(t *testing.T) {
	t.Parallel()
	blob := strings.Repeat("QUJD", 64)
	body := `{"model":"claude-x","messages":[{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"ana.souza@correio.pt","signature":"sig52998224725"},` +
		`{"type":"redacted_thinking","data":"` + blob + `"},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + blob + `"}},` +
		`{"type":"tool_use","id":"toolu_52998224725","name":"send_52998224725","input":{}}]}],` +
		`"tools":[{"name":"send_52998224725","input_schema":{"type":"object","properties":{"cpf":{"enum":["52998224725"]}}}}]}`
	out, err := New().MaskRequest(nil, []byte(body), session(t))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(body, `"enum":["52998224725"]`, `"enum":["[CPF_1S1AAT]"]`, 1)
	if string(out) != want {
		t.Fatalf("only the enum value may change:\n%s\n%s", want, out)
	}
}

func TestStopSequencesAndSchemaTextMasked(t *testing.T) {
	t.Parallel()
	body := `{"messages":[],"stop_sequences":["ana.souza@correio.pt"],"tools":[{"name":"x","input_schema":` +
		`{"type":"object","properties":{"to":{"type":"string","description":"ana.souza@correio.pt","pattern":"^.+@.+$"}}}}]}`
	out, err := New().MaskRequest(nil, []byte(body), session(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "ana.souza@correio.pt") || !strings.Contains(string(out), `"pattern":"^.+@.+$"`) {
		t.Fatalf("got %s", out)
	}
}

func masked(t *testing.T, s *mask.Session, text string) string {
	t.Helper()
	out, err := s.MaskText(nil, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

type evt struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		Thinking    string `json:"thinking"`
	} `json:"delta"`
}

func emit(sb *strings.Builder, name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sb.WriteString("event: " + name + "\ndata: " + string(b) + "\n\n")
}

func chunked(rng *rand.Rand, s string) []string {
	var out []string
	for i := 0; i < len(s); {
		n := min(1+rng.IntN(7), len(s)-i)
		out = append(out, s[i:i+n])
		i += n
	}
	return out
}

func buildStream(rng *rand.Rand, text, args string, restart bool) string {
	var sb strings.Builder
	emit(
		&sb,
		evMessageStart,
		map[string]any{"type": evMessageStart, "message": map[string]any{"id": "m", "content": []any{}}},
	)
	emit(
		&sb,
		evBlockStart,
		map[string]any{
			"type":          evBlockStart,
			"index":         0,
			"content_block": map[string]any{"type": "thinking", "thinking": ""},
		},
	)
	emit(
		&sb,
		evBlockDelta,
		map[string]any{
			"type":  evBlockDelta,
			"index": 0,
			"delta": map[string]any{"type": "thinking_delta", "thinking": "[EMAIL_ZZZZZZ]"},
		},
	)
	emit(&sb, evBlockStop, map[string]any{"type": evBlockStop, "index": 0})
	emit(
		&sb,
		evBlockStart,
		map[string]any{"type": evBlockStart, "index": 1, "content_block": map[string]any{"type": "text", "text": ""}},
	)
	for _, c := range chunked(rng, text) {
		emit(
			&sb,
			evBlockDelta,
			map[string]any{"type": evBlockDelta, "index": 1, "delta": map[string]any{"type": deltaText, "text": c}},
		)
	}
	emit(&sb, evBlockStop, map[string]any{"type": evBlockStop, "index": 1})
	idx := 2
	if restart {
		idx = 1
	}
	emit(
		&sb,
		evBlockStart,
		map[string]any{
			"type":          evBlockStart,
			"index":         idx,
			"content_block": map[string]any{"type": "tool_use", "id": "t", "name": "x", "input": map[string]any{}},
		},
	)
	for _, c := range chunked(rng, args) {
		emit(
			&sb,
			evBlockDelta,
			map[string]any{
				"type":  evBlockDelta,
				"index": idx,
				"delta": map[string]any{"type": deltaJSON, "partial_json": c},
			},
		)
	}
	emit(&sb, evBlockStop, map[string]any{"type": evBlockStop, "index": idx})
	emit(&sb, evMessageStop, map[string]any{"type": evMessageStop})
	return sb.String()
}

func collect(t *testing.T, r io.Reader) (text, args, thinking string) {
	t.Helper()
	var tb, ab, hb strings.Builder
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var e evt
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			t.Fatalf("bad event %q", data)
		}
		if e.Delta == nil {
			continue
		}
		tb.WriteString(e.Delta.Text)
		ab.WriteString(e.Delta.PartialJSON)
		hb.WriteString(e.Delta.Thinking)
	}
	return tb.String(), ab.String(), hb.String()
}

func TestStreamRestoreRandomChunking(t *testing.T) {
	t.Parallel()
	original := "Reply to ana.souza@correio.pt about CPF 529.982.247-25."
	for seed := range uint64(100) {
		s := session(t)
		m := masked(t, s, original)
		args := `{"to":"` + masked(t, s, "ana.souza@correio.pt") + `"}`
		rng := rand.New(rand.NewPCG(seed, 7))
		body := NewStreamBody(
			io.NopCloser(strings.NewReader(buildStream(rng, "Echo: "+m, args, seed%2 == 1))),
			s,
			codec.StreamOptions{},
		)
		text, gotArgs, thinking := collect(t, body)
		if text != "Echo: "+original || gotArgs != `{"to":"ana.souza@correio.pt"}` || thinking != "[EMAIL_ZZZZZZ]" {
			t.Fatalf("seed %d: text=%q args=%q thinking=%q", seed, text, gotArgs, thinking)
		}
		if err := body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRestoreBodyAndSynthesize(t *testing.T) {
	t.Parallel()
	s := session(t)
	tok := masked(t, s, "ana.souza@correio.pt")
	body := `{"id":"m","type":"message","role":"assistant","model":"x","content":[{"type":"text","text":"Hi ` + tok +
		`"},{"type":"tool_use","id":"t","name":"x","input":{"to":"` + tok + `"}}],"stop_reason":"end_turn","stop_sequence":null}`
	out, st, err := New().RestoreBody(nil, []byte(body), s, restoreOpts())
	if err != nil || st.Restored != 2 {
		t.Fatalf("restore %v %+v", err, st)
	}
	sse, err := Synthesize(nil, out)
	if err != nil {
		t.Fatal(err)
	}
	text, args, _ := collect(t, bytes.NewReader(sse))
	if text != "Hi ana.souza@correio.pt" || args != `{"to":"ana.souza@correio.pt"}` {
		t.Fatalf("synth text=%q args=%q\n%s", text, args, sse)
	}
	if !strings.Contains(string(sse), "event: message_stop") {
		t.Fatal("missing message_stop")
	}
	for _, want := range []string{`"id":"t"`, `"name":"x"`, `"id":"m"`, `"model":"x"`, `"stop_reason":"end_turn"`} {
		if !strings.Contains(string(sse), want) {
			t.Fatalf("synthesized stream lost %s:\n%s", want, sse)
		}
	}
}

func TestStreamPreservesUnknownFields(t *testing.T) {
	t.Parallel()
	s := session(t)
	tok := masked(t, s, "ana.souza@correio.pt")
	in := "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi " + tok + "\",\"extra_field\":7},\"trace\":\"abc\"}\n\n"
	out, err := io.ReadAll(NewStreamBody(io.NopCloser(strings.NewReader(in)), s, codec.StreamOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"text":"Hi ana.souza@correio.pt"`, `"extra_field":7`, `"trace":"abc"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(string(out), "Extra") {
		t.Fatalf("leaked Go field name:\n%s", out)
	}
}
