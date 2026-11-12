package openai

import (
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
)

func benchPrompt(size int) string {
	pii := "Customer Ana Souza (ana.souza@correio.pt, CPF 529.982.247-25, tel +351 912 345 678) asked about " +
		"the refund to IBAN PT50 0002 0123 1234 5678 9015 4 from host 8.8.8.8. "
	filler := "Please summarise the ticket history, keep a neutral tone, list the next actions with owners and " +
		"deadlines, and avoid promising anything the policy does not allow. "
	var sb strings.Builder
	sb.WriteString(pii)
	for sb.Len() < size {
		sb.WriteString(filler)
	}
	return sb.String()[:size]
}

func benchBody(size int) []byte {
	q := jsonx.Quote(nil, []byte(benchPrompt(size)))
	return []byte(`{"model":"gpt-4.1-mini","messages":[{"role":"system","content":"You are a support assistant."},` +
		`{"role":"user","content":` + string(q) + `}]}`)
}

func BenchmarkMaskRequest(b *testing.B) {
	c := New()
	body := benchBody(4096)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	out := make([]byte, 0, len(body)*2)
	eng, tn := engineAndTenant(b)
	for b.Loop() {
		s := eng.NewSession(tn)
		var err error
		out, err = c.MaskRequest(out[:0], body, s)
		if err != nil {
			b.Fatal(err)
		}
		s.Release()
	}
}

func BenchmarkRestoreBody(b *testing.B) {
	c := New()
	s := session(b)
	masked, err := s.MaskText(nil, []byte(benchPrompt(4096)))
	if err != nil {
		b.Fatal(err)
	}
	body := []byte(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":` +
		string(jsonx.Quote(nil, masked)) + `},"finish_reason":"stop"}]}`)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	out := make([]byte, 0, len(body)*2)
	for b.Loop() {
		out, _, err = c.RestoreBody(out[:0], body, s, restore.Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
