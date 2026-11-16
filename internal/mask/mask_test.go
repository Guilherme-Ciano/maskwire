package mask

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

func newTenant(tb testing.TB, id string, p *policy.Policy) *Tenant {
	tb.Helper()
	kr, err := token.NewKeyring(bytes.Repeat([]byte{7}, 32), 1)
	if err != nil {
		tb.Fatal(err)
	}
	tk, err := kr.Tenant(id)
	if err != nil {
		tb.Fatal(err)
	}
	return &Tenant{ID: id, Key: tk, Policy: p}
}

func mustMask(t *testing.T, s *Session, text string) []byte {
	t.Helper()
	out, err := s.MaskText(nil, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newEngine() *Engine {
	return NewEngine(detect.New(detect.Options{}), 0)
}

const sample = "Olá, sou a Ana (ana.silva@empresa.com.br), CPF 529.982.247-25, IBAN PT50 0002 0123 1234 5678 9015 4, ip 8.8.8.8."

func TestMaskRestoreIdentity(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	s := eng.NewSession(newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask)))
	masked, err := s.MaskText(nil, []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"ana.silva", "529.982", "PT50", "8.8.8.8"} {
		if strings.Contains(string(masked), leak) {
			t.Fatalf("leak %q in %s", leak, masked)
		}
	}
	if s.Issued() != 4 {
		t.Fatalf("issued %d tokens: %s", s.Issued(), masked)
	}
	back, st := restore.Restore(nil, masked, s, restore.Options{})
	if string(back) != sample || st.Restored != 4 {
		t.Fatalf("restore mismatch: %s (%+v)", back, st)
	}
}

func TestDeterminismAndTenantSeparation(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	p := policy.New(policy.Enforce, policy.Mask)
	a, b := newTenant(t, "acme", p), newTenant(t, "globex", p)
	var wg sync.WaitGroup
	outs := make([][]byte, 8)
	errs := make([]error, 8)
	for i := range outs {
		wg.Go(func() {
			s := eng.NewSession(a)
			outs[i], errs[i] = s.MaskText(nil, []byte(sample))
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	for _, o := range outs[1:] {
		if !bytes.Equal(o, outs[0]) {
			t.Fatalf("non-deterministic: %s vs %s", o, outs[0])
		}
	}
	other := mustMask(t, eng.NewSession(b), sample)
	if bytes.Equal(other, outs[0]) {
		t.Fatal("tenants must not share tokens")
	}
	canonA := mustMask(t, eng.NewSession(a), "CPF 52998224725")
	canonB := mustMask(t, eng.NewSession(a), "CPF 529.982.247-25")
	canonC := mustMask(
		t,
		eng.NewSession(a),
		"CPF \uff15\uff12\uff19.\uff19\uff18\uff12.\uff12\uff14\uff17-\uff12\uff15",
	)
	if !bytes.Equal(canonA, canonC) {
		t.Fatalf("fullwidth digits must canonicalise like ASCII: %s vs %s", canonA, canonC)
	}
	if !bytes.Equal(canonA, canonB) {
		t.Fatalf("canonical forms differ: %s vs %s", canonA, canonB)
	}
}

func TestPolicies(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	p := policy.New(policy.Enforce, policy.Mask)
	p.Set(entity.Email, policy.Rule{Action: policy.Redact})
	p.Set(entity.IP, policy.Rule{Action: policy.Allow})
	s := eng.NewSession(newTenant(t, "acme", p))
	out, err := s.MaskText(nil, []byte("mail a@b.pt ip 8.8.8.8 cpf 52998224725"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "[REDACTED_EMAIL]") || !strings.Contains(string(out), "8.8.8.8") ||
		strings.Contains(string(out), "52998224725") {
		t.Fatalf("unexpected: %s", out)
	}
	p.Set(entity.CPF, policy.Rule{Action: policy.Block})
	s = eng.NewSession(newTenant(t, "acme", p))
	if _, err = s.MaskText(nil, []byte("cpf 52998224725")); err != nil {
		t.Fatal(err)
	}
	var be *BlockedError
	if !errors.As(s.Err(), &be) || len(be.Types) != 1 || be.Types[0] != entity.CPF {
		t.Fatalf("expected block, got %v", s.Err())
	}
	if strings.Contains(be.Error(), "529") {
		t.Fatal("error must not contain the value")
	}
}

func TestObserveLeavesTextUntouched(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	s := eng.NewSession(newTenant(t, "acme", policy.New(policy.Observe, policy.Block)))
	out, err := s.MaskText(nil, []byte(sample))
	if err != nil || string(out) != sample || s.Err() != nil {
		t.Fatalf("observe changed output: %s %v", out, s.Err())
	}
	if s.Counters().TotalDetected() != 4 {
		t.Fatalf("detected %d", s.Counters().TotalDetected())
	}
}

func TestCollisionExtendsSuffix(t *testing.T) {
	t.Parallel()
	eng := NewEngine(detect.New(detect.Options{}), 50000)
	tn := newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask))
	tn.TokenLength = 5
	s := eng.NewSession(tn)
	var sb strings.Builder
	for i := range 20000 {
		sb.WriteString("u")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("@corp.pt ")
	}
	masked, err := s.MaskText(nil, []byte(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`\[EMAIL_[0-9A-Z]{7}\]`).Match(masked) {
		t.Fatal("expected at least one extended suffix among 20000 values at 25 bits")
	}
	back, st := restore.Restore(nil, masked, s, restore.Options{})
	if string(back) != sb.String() || st.Foreign != 0 || st.Restored != 20000 {
		t.Fatalf("collision handling broke restore: restored=%d foreign=%d", st.Restored, st.Foreign)
	}
}

func TestMaskJSONStrings(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	s := eng.NewSession(newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask)))
	doc := `{"to":"ana@empresa.pt","n":52998224725,"list":["cpf 52998224725"]}`
	out, err := s.MaskJSONStrings(nil, []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "ana@") || !strings.Contains(string(out), `"n":"[CPF_`) ||
		strings.Contains(string(out), "cpf 5299") {
		t.Fatalf("unexpected %s", out)
	}
}

func TestNumericLeafIsMarked(t *testing.T) {
	t.Parallel()
	s := newEngine().NewSession(newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask)))
	out, err := s.MaskJSONStrings(nil, []byte(`{"cpf":52998224725,"note":"cpf 11144477735"}`))
	if err != nil {
		t.Fatal(err)
	}
	back, _ := restore.Restore(nil, out, s, restore.Options{Encoding: restore.JSONString})
	if string(back) != `{"cpf":52998224725,"note":"cpf 11144477735"}` {
		t.Fatalf("masked %s restored %s", out, back)
	}
}

func TestPropertyMaskThenRestore(t *testing.T) {
	t.Parallel()
	eng := newEngine()
	tn := newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask))
	pieces := []string{
		"ana@empresa.pt",
		"529.982.247-25",
		"11.222.333/0001-81",
		"PT50 0002 0123 1234 5678 9015 4",
		"4242 4242 4242 4242",
		"RSSMRA85T10A562S",
		"12345678Z",
		"8.8.8.8",
		" ",
		", ",
		"olá ",
		"texto ",
		"[",
		"]",
		"_",
		"\n",
		"[EMAIL_",
	}
	rapid.Check(t, func(rt *rapid.T) {
		src := strings.Join(rapid.SliceOf(rapid.SampledFrom(pieces)).Draw(rt, "parts"), "")
		s := eng.NewSession(tn)
		masked, err := s.MaskText(nil, []byte(src))
		if err != nil {
			rt.Fatal(err)
		}
		back, _ := restore.Restore(nil, masked, s, restore.Options{})
		if s.SurfaceVariants() > 0 {
			return
		}
		if string(back) != src {
			rt.Fatalf("round trip\n src %q\nmask %q\nback %q", src, masked, back)
		}
		again, err := eng.NewSession(tn).MaskText(nil, back)
		if err != nil {
			rt.Fatal(err)
		}
		if !bytes.Equal(again, masked) {
			rt.Fatalf("restore changed meaning\n src %q\nmask %q\nback %q\nagain %q", src, masked, back, again)
		}
	})
}

func TestSurfaceVariantsRestoreFirstForm(t *testing.T) {
	t.Parallel()
	s := newEngine().NewSession(newTenant(t, "acme", policy.New(policy.Enforce, policy.Mask)))
	masked := mustMask(t, s, "CPF 529.982.247-25 or 52998224725")
	back, _ := restore.Restore(nil, masked, s, restore.Options{})
	if s.SurfaceVariants() != 1 || string(back) != "CPF 529.982.247-25 or 529.982.247-25" {
		t.Fatalf("variants=%d back=%q", s.SurfaceVariants(), back)
	}
}

func FuzzMaskRoundTrip(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte("[CPF_ABCDEF] 529.982.247-25"))
	eng := newEngine()
	tn := newTenant(f, "acme", policy.New(policy.Enforce, policy.Mask))
	f.Fuzz(func(t *testing.T, src []byte) {
		if bytes.IndexByte(src, '[') >= 0 || bytes.IndexByte(src, '\\') >= 0 {
			return
		}
		s := eng.NewSession(tn)
		masked, err := s.MaskText(nil, src)
		if err != nil {
			return
		}
		back, _ := restore.Restore(nil, masked, s, restore.Options{})
		if s.SurfaceVariants() == 0 && !bytes.Equal(back, src) {
			t.Fatalf("round trip\n src %q\nmask %q\nback %q", src, masked, back)
		}
	})
}
