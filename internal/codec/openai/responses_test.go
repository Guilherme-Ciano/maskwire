package openai

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"io"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
)

func readResponsesText(t *testing.T, r io.Reader) (deltas, done string) {
	t.Helper()
	var sb strings.Builder
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("bad event %q", data)
		}
		switch ev.Type {
		case evTextDelta:
			sb.WriteString(ev.Delta)
		case evTextDone:
			done = ev.Text
		}
	}
	return sb.String(), done
}

func TestResponsesStreamFlushesBeforeDone(t *testing.T) {
	t.Parallel()
	s := session(t)
	tok, err := s.MaskText(nil, []byte("ana.souza@correio.pt"))
	if err != nil {
		t.Fatal(err)
	}
	half := len(tok) / 2
	in := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"m\",\"output_index\":0,\"content_index\":0,\"delta\":\"Hi " + string(
		tok[:half],
	) + "\",\"obfuscation\":\"xy\"}\n\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"m\",\"output_index\":0,\"content_index\":0,\"delta\":\"" + string(
		tok[half:],
	) + " [EMA\"}\n\n" +
		"event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"item_id\":\"m\",\"output_index\":0,\"content_index\":0,\"text\":\"Hi " + string(
		tok,
	) + " [EMA\"}\n\n"
	out, err := io.ReadAll(New().NewResponsesStream(io.NopCloser(strings.NewReader(in)), s, codec.StreamOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	deltas, done := readResponsesText(t, bytes.NewReader(out))
	if deltas != "Hi ana.souza@correio.pt [EMA" || done != "Hi ana.souza@correio.pt [EMA" {
		t.Fatalf("deltas %q done %q\n%s", deltas, done, out)
	}
	if !bytes.Contains(out, []byte(`"obfuscation":"xy"`)) {
		t.Fatalf("unknown fields must survive:\n%s", out)
	}
}

func TestSynthesizeResponses(t *testing.T) {
	t.Parallel()
	body := `{"id":"r","object":"response","output":[{"type":"message","id":"m","content":[{"type":"output_text","text":"hello"}]}]}`
	out, err := SynthesizeResponses(nil, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	deltas, done := readResponsesText(t, bytes.NewReader(out))
	if deltas != "hello" || done != "hello" || !bytes.Contains(out, []byte("event: response.completed")) {
		t.Fatalf("synth:\n%s", out)
	}
}
