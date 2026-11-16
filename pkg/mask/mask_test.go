package mask

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const text = "Escreva para ana.silva@empresa.com.br (CPF 529.982.247-25), IBAN PT50 0002 0123 1234 5678 9015 4."

func newMasker(t *testing.T, o Options) *Masker {
	t.Helper()
	o.MasterKey = bytes.Repeat([]byte{7}, 32)
	if o.Tenant == "" {
		o.Tenant = "acme"
	}
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMaskAndRestore(t *testing.T) {
	m := newMasker(t, Options{})
	s := m.Session()
	defer s.Close()
	masked, err := s.Mask(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"ana.silva", "529.982.247-25", "PT50"} {
		if strings.Contains(masked, raw) {
			t.Fatalf("%q left in %s", raw, masked)
		}
	}
	if got := s.Restore("Reply: " + masked); got != "Reply: "+text {
		t.Fatalf("restore: %s", got)
	}
	if c := s.Counts(); c["EMAIL"] != 1 || c["CPF"] != 1 || c["IBAN"] != 1 {
		t.Fatalf("counts %v", c)
	}
	r := s.Restorer()
	var out []byte
	for i := range len(masked) {
		out = r.Write(out, []byte{masked[i]})
	}
	if out = r.Flush(out); string(out) != text {
		t.Fatalf("stream restore: %s", out)
	}
	again := m.Session()
	defer again.Close()
	if m2, err := again.Mask(text); err != nil || m2 != masked {
		t.Fatal("tokens are not deterministic across sessions")
	}
	if got := again.Restore("[EMAIL_ZZZZZZ]"); got != "[email]" {
		t.Fatalf("foreign token: %s", got)
	}
}

func TestMaskJSONAndBlock(t *testing.T) {
	m := newMasker(t, Options{Rules: map[string]Rule{"CREDIT_CARD": {Action: "block"}}})
	s := m.Session()
	defer s.Close()
	doc, err := s.MaskJSON([]byte(`{"msg":"mail ana.silva@empresa.com.br"}`))
	if err != nil || bytes.Contains(doc, []byte("ana.silva")) {
		t.Fatalf("%s %v", doc, err)
	}
	blocked := m.Session()
	defer blocked.Close()
	_, err = blocked.Mask("card 4242 4242 4242 4242")
	var b *BlockedError
	if !errors.As(err, &b) || len(b.Types) != 1 || b.Types[0] != "CREDIT_CARD" ||
		!strings.Contains(b.Error(), "CREDIT_CARD") {
		t.Fatalf("blocked: %v", err)
	}
}

func TestOptionsAndDetect(t *testing.T) {
	for _, o := range []Options{
		{Tenant: "", MasterKey: bytes.Repeat([]byte{1}, 32)},
		{Tenant: "a", MasterKey: []byte("short")},
		{Tenant: "a", MasterKey: bytes.Repeat([]byte{1}, 32), Default: "explode"},
		{Tenant: "a", MasterKey: bytes.Repeat([]byte{1}, 32), TokenLength: 3},
	} {
		if _, err := New(o); err == nil {
			t.Errorf("options %+v accepted", o)
		}
	}
	got := Detect("CPF 529.982.247-25")
	if len(got) != 1 || got[0].Type != "CPF" || got[0].Score != 1 {
		t.Fatalf("detect %v", got)
	}
}
