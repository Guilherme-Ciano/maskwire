package gemini

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
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
	kr, err := token.NewKeyring(bytes.Repeat([]byte{7}, 32), 1)
	if err != nil {
		tb.Fatal(err)
	}
	tk, err := kr.Tenant("acme")
	if err != nil {
		tb.Fatal(err)
	}
	return mask.NewEngine(detect.New(detect.Options{}), 0).NewSession(
		&mask.Tenant{ID: "acme", Key: tk, Policy: policy.New(policy.Enforce, policy.Mask)})
}

func masked(tb testing.TB, s *mask.Session, text string) string {
	tb.Helper()
	out, err := s.MaskText(nil, []byte(text))
	if err != nil {
		tb.Fatal(err)
	}
	return string(out)
}

const request = `{
 "contents":[
  {"role":"user","parts":[
   {"text":"Email ana.souza@correio.pt about CPF 529.982.247-25"},
   {"inlineData":{"mimeType":"image/png","data":"ana.souza@correio.pt"}},
   {"fileData":{"mimeType":"application/pdf","fileUri":"gs://b/ana.souza@correio.pt"}}]},
  {"role":"model","parts":[{"functionCall":{"name":"send_ana.souza@correio.pt","args":{"to":"ana.souza@correio.pt","cpf":"52998224725","n":52998224725}}}]},
  {"role":"user","parts":[{"functionResponse":{"name":"send","response":{"status":"sent to ana.souza@correio.pt"}}}]}],
 "systemInstruction":{"role":"system","parts":[{"text":"The customer is ana.souza@correio.pt"}]},
 "tools":[{"functionDeclarations":[{"name":"send","description":"write to ana.souza@correio.pt",
   "parameters":{"type":"object","properties":{"to":{"type":"string","description":"like ana.souza@correio.pt","enum":["ana.souza@correio.pt"]}}}}]}],
 "generationConfig":{"stopSequences":["ana.souza@correio.pt"],"responseMimeType":"application/json",
   "responseSchema":{"type":"object","description":"for ana.souza@correio.pt"}},
 "safetySettings":[{"category":"HARM_CATEGORY_HARASSMENT","threshold":"BLOCK_NONE"}],
 "cachedContent":"cachedContents/ana.souza@correio.pt"
}`

func TestMaskRequestCoversShapes(t *testing.T) {
	t.Parallel()
	s := session(t)
	out, err := New().MaskRequest(nil, []byte(request), s)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("masked request is not JSON: %v\n%s", err, out)
	}
	text := string(out)
	for _, kept := range []string{
		`"data":"ana.souza@correio.pt"`,
		`"fileUri":"gs://b/ana.souza@correio.pt"`,
		`"name":"send_ana.souza@correio.pt"`,
		`"cachedContent":"cachedContents/ana.souza@correio.pt"`,
		`"responseMimeType":"application/json"`,
		`"threshold":"BLOCK_NONE"`,
		`"type":"object"`,
	} {
		if !strings.Contains(text, kept) {
			t.Errorf("must stay untouched: %s", kept)
		}
	}
	if n := strings.Count(text, "ana.souza@correio.pt"); n != 4 {
		t.Errorf("expected only the 4 protected fields to keep the e-mail, found %d:\n%s", n, out)
	}
	for _, gone := range []string{"529.982.247-25", `"cpf":"52998224725"`, `"n":52998224725`} {
		if strings.Contains(text, gone) {
			t.Errorf("leaked %s", gone)
		}
	}
}

func TestRestoreBody(t *testing.T) {
	t.Parallel()
	s := session(t)
	email, cpf := masked(t, s, "ana.souza@correio.pt"), masked(t, s, "CPF 529.982.247-25")
	body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Write to ` + email + ` about ` + cpf +
		`"},{"functionCall":{"name":"send","args":{"to":"` + email + `"}}}]},"finishReason":"STOP","index":0}],` +
		`"usageMetadata":{"promptTokenCount":3},"modelVersion":"` + email + `"}`
	out, st, err := New().RestoreBody(nil, []byte(body), s, restore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := `Write to ana.souza@correio.pt about CPF 529.982.247-25`
	if !strings.Contains(string(out), want) || !strings.Contains(string(out), `"to":"ana.souza@correio.pt"`) {
		t.Fatalf("not restored:\n%s", out)
	}
	if !strings.Contains(string(out), `"modelVersion":"`+email+`"`) {
		t.Fatal("metadata outside candidates must not be restored")
	}
	if st.Restored != 3 {
		t.Fatalf("restored %d", st.Restored)
	}
}

func streamOf(rng *rand.Rand, text, args string) []byte {
	var src strings.Builder
	for i := 0; i < len(text); {
		n := 1 + rng.IntN(7)
		part := jsonx.Quote(nil, []byte(text[i:min(i+n, len(text))]))
		i += n
		fmt.Fprintf(
			&src,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%s}],\"role\":\"model\"},\"index\":0}],"+
				"\"usageMetadata\":{\"promptTokenCount\":3},\"modelVersion\":\"gemini-2.5-flash\"}\r\n\r\n",
			part,
		)
	}
	if args != "" {
		fmt.Fprintf(
			&src,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"send\",\"args\":{\"to\":%s}}}],\"role\":\"model\"},\"index\":0}]}\r\n\r\n",
			jsonx.Quote(nil, []byte(args)),
		)
	}
	switch rng.IntN(3) {
	case 0:
		src.WriteString(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}]}\r\n\r\n",
		)
	case 1:
		src.WriteString(
			"data: {\"candidates\":[{\"content\":{\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}]}\r\n\r\n",
		)
	}
	return []byte(src.String())
}

type geminiEvent struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text         string `json:"text"`
				FunctionCall *struct {
					Args map[string]string `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func collect(t *testing.T, out []byte) (text string, args []string) {
	t.Helper()
	var b strings.Builder
	for line := range strings.SplitSeq(string(out), "\n") {
		data, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "data: ")
		if !ok {
			continue
		}
		var ev geminiEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("event is not JSON: %v\n%s", err, data)
		}
		for _, c := range ev.Candidates {
			for _, p := range c.Content.Parts {
				b.WriteString(p.Text)
				if p.FunctionCall != nil {
					args = append(args, p.FunctionCall.Args["to"])
				}
			}
		}
	}
	return b.String(), args
}

func TestStreamRestoreRandomChunking(t *testing.T) {
	t.Parallel()
	s := session(t)
	email, cpf := masked(t, s, "ana.souza@correio.pt"), masked(t, s, "CPF 529.982.247-25")
	text := "Reply to " + email + " about " + cpf + ". Thanks, " + email
	want := "Reply to ana.souza@correio.pt about CPF 529.982.247-25. Thanks, ana.souza@correio.pt"
	for seed := range uint64(400) {
		rng := rand.New(rand.NewPCG(seed, 3))
		src := streamOf(rng, text, email)
		body := NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{})
		out, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		got, args := collect(t, out)
		if got != want {
			t.Fatalf("seed %d: text %q\n%s", seed, got, out)
		}
		if len(args) != 1 || args[0] != "ana.souza@correio.pt" {
			t.Fatalf("seed %d: args %v", seed, args)
		}
		if st := body.Stats(); st.Restored != 4 {
			t.Fatalf("seed %d: restored %d", seed, st.Restored)
		}
	}
}

func TestStreamKeepsOtherEventsAndComments(t *testing.T) {
	t.Parallel()
	src := ": ping\n\ndata: not json\n\ndata: {\"usageMetadata\":{\"totalTokenCount\":9}}\n\n"
	out, err := io.ReadAll(NewStreamBody(io.NopCloser(strings.NewReader(src)), session(t), codec.StreamOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{": ping", "data: not json", `"totalTokenCount":9`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestInjectHint(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
		`{"systemInstruction":{"parts":[{"text":"be brief"}]},"contents":[]}`,
	} {
		out, err := InjectHint([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			SystemInstruction struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"systemInstruction"`
		}
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if len(doc.SystemInstruction.Parts) == 0 || doc.SystemInstruction.Parts[0].Text != codec.PromptHint {
			t.Fatalf("hint not first part:\n%s", out)
		}
	}
}

func TestParseMeta(t *testing.T) {
	t.Parallel()
	if _, err := ParseMeta([]byte(`{"contents":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMeta([]byte(`{"contents":`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
