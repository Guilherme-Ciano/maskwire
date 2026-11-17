package restore

import (
	"bytes"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type fakeSet map[string][]byte

func (f fakeSet) Numeric(suffix []byte) bool {
	return string(suffix) == "N0MBER"
}

func (f fakeSet) Kind(suffix []byte) (entity.Type, bool) {
	for k := range f {
		label, sfx, _ := strings.Cut(k, "_")
		if sfx != string(suffix) {
			continue
		}
		for _, t := range entity.All() {
			if t.Label() == label {
				return t, true
			}
		}
	}
	return entity.Unknown, false
}

func (f fakeSet) Lookup(t entity.Type, suffix []byte) ([]byte, bool) {
	if t == entity.Unknown {
		for k, v := range f {
			if strings.HasSuffix(k, "_"+string(suffix)) {
				return v, true
			}
		}
		return nil, false
	}
	var key [32]byte
	k := append(append(append(key[:0], t.Label()...), '_'), suffix...)
	v, ok := f[string(k)]
	return v, ok
}

var set = fakeSet{
	"EMAIL_K7Q2MA": []byte("ana@empresa.pt"),
	"CPF_7H3JX9":   []byte("529.982.247-25"),
	"IP_ABCDE":     []byte("8.8.8.8"),
	"EMAIL_QX0TE1": []byte(`o"brien\x@x.pt`),
	"CPF_N0MBER":   []byte("52998224725"),
}

func TestRestore(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"hi [EMAIL_K7Q2MA]!", "hi ana@empresa.pt!"},
		{"lower [email_k7q2ma] case", "lower ana@empresa.pt case"},
		{`md \[EMAIL\_K7Q2MA\] esc`, "md ana@empresa.pt esc"},
		{"spaced [ EMAIL_K7Q2MA ] ok", "spaced ana@empresa.pt ok"},
		{"double [[EMAIL_K7Q2MA]]", "double [ana@empresa.pt]"},
		{"two [CPF_7H3JX9][IP_ABCDE]", "two 529.982.247-258.8.8.8"},
		{"foreign [EMAIL_ZZZZZZ] x", "foreign [email] x"},
		{"wrong label [CPF_K7Q2MA]", "wrong label [cpf]"},
		{"redacted [REDACTED_EMAIL] stays", "redacted [REDACTED_EMAIL] stays"},
		{"unknown label [FOO_K7Q2MA] restores", "unknown label ana@empresa.pt restores"},
		{"unknown label [FOO_ZZZZZZ] stays", "unknown label [FOO_ZZZZZZ] stays"},
		{"short [EMAIL_K7Q] stays", "short [EMAIL_K7Q] stays"},
		{"bad crock [EMAIL_K7Q2MI]", "bad crock [EMAIL_K7Q2MI]"},
		{"trailing [EMAIL_K7Q2", "trailing [EMAIL_K7Q2"},
		{"lone \\ backslash [", "lone \\ backslash ["},
	}
	for _, tc := range cases {
		got, _ := Restore(nil, []byte(tc.in), set, Options{})
		if string(got) != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestRelaxedVariants(t *testing.T) {
	t.Parallel()
	restored := []string{
		"EMAIL_K7Q2MA", "[EMAIL K7Q2MA]", "[EMAIL-K7Q2MA]", "(EMAIL_K7Q2MA)", "<EMAIL_K7Q2MA>", "{EMAIL_K7Q2MA}",
		"\u3010EMAIL_K7Q2MA\u3011", "\u3014EMAIL_K7Q2MA\u3015", "[E-MAIL_K7Q2MA]", "[E-mail_K7Q2MA]", "[CORREO_K7Q2MA]",
		"[EMAIL__K7Q2MA]", "%5BEMAIL_K7Q2MA%5D", "%5bemail_k7q2ma%5d", "[EMAIL_K7Q2MA", "EMAIL K7Q2MA",
	}
	for _, v := range restored {
		in := "to " + v + " today"
		got, _ := Restore(nil, []byte(in), set, Options{})
		if string(got) != "to ana@empresa.pt today" {
			t.Errorf("%q: got %q", in, got)
		}
	}
	untouched := []string{
		"EMAIL ADDRESS please", "THE REPORT IS READY", "Hello World", "[EMAIL ADDRESS]", "EMAIL_ZZZZZZ",
		"xEMAIL_K7Q2MA", "EMAIL_K7Q2MAX", "(see EMAIL_ZZZZZZ)", "[REDACTED_EMAIL]", "email_k7q2ma",
	}
	for _, v := range untouched {
		got, st := Restore(nil, []byte(v), set, Options{})
		if string(got) != v || st.Foreign != 0 {
			t.Errorf("%q must stay: got %q (%+v)", v, got, st)
		}
	}
	got, _ := Restore(nil, []byte(`{"to":"\u005bEMAIL_K7Q2MA\u005d"}`), set, Options{Encoding: JSONString})
	if string(got) != `{"to":"ana@empresa.pt"}` {
		t.Errorf("json unicode escape: got %s", got)
	}
}

func TestNumericTokensRestoreAsNumbers(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`{"cpf":"[CPF_N0MBER]"}`:         `{"cpf":52998224725}`,
		`{"cpf": "[CPF_N0MBER]" }`:       `{"cpf": 52998224725 }`,
		`{"note":"CPF [CPF_N0MBER] ok"}`: `{"note":"CPF 52998224725 ok"}`,
		`{"to":"[EMAIL_K7Q2MA]"}`:        `{"to":"ana@empresa.pt"}`,
		`{"ids":["[CPF_N0MBER]","x"]}`:   `{"ids":[52998224725,"x"]}`,
		`{"cpf":"CPF_N0MBER"}`:           `{"cpf":52998224725}`,
	}
	for in, want := range cases {
		got, _ := Restore(nil, []byte(in), set, Options{Encoding: JSONString})
		if string(got) != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
		for i := 0; i <= len(in); i++ {
			if split, _ := streamed([]byte(in), []int{i}, Options{Encoding: JSONString}); string(split) != want {
				t.Fatalf("%s split at %d: got %s", in, i, split)
			}
		}
	}
	if got, _ := Restore(nil, []byte(`"[CPF_N0MBER]"`), set, Options{}); string(got) != `"52998224725"` {
		t.Fatalf("raw text keeps its quotes: %s", got)
	}
}

func TestBareTokenAtEndOfStream(t *testing.T) {
	t.Parallel()
	s := NewStream(set, Options{})
	out := s.Feed(nil, []byte("contact EMAIL_K7"))
	out = s.Feed(out, []byte("Q2MA"))
	out = s.Flush(out)
	if string(out) != "contact ana@empresa.pt" {
		t.Fatalf("got %q", out)
	}
}

func TestForeignKeep(t *testing.T) {
	t.Parallel()
	got, st := Restore(nil, []byte("x [EMAIL_ZZZZZZ]"), set, Options{Foreign: ForeignKeep})
	if string(got) != "x [EMAIL_ZZZZZZ]" || st.Foreign != 1 {
		t.Fatalf("got %q %+v", got, st)
	}
}

func TestJSONStringEncoding(t *testing.T) {
	t.Parallel()
	got, _ := Restore(nil, []byte(`{"to":"[EMAIL_QX0TE1]"}`), set, Options{Encoding: JSONString})
	want := `{"to":"o\"brien\\x@x.pt"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func streamed(src []byte, cuts []int, o Options) (result []byte, held int) {
	s := NewStream(set, o)
	var out []byte
	prev := 0
	maxHeld := 0
	for _, c := range cuts {
		out = s.Feed(out, src[prev:c])
		maxHeld = max(maxHeld, s.Held())
		prev = c
	}
	out = s.Feed(out, src[prev:])
	maxHeld = max(maxHeld, s.Held())
	return s.Flush(out), maxHeld
}

func TestSplitAtEveryByte(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"Dear [EMAIL_K7Q2MA], your CPF [CPF_7H3JX9] and ip \\[IP\\_ABCDE\\] were seen; foreign [EMAIL_ZZZZZZ] [[",
		"[[[EMAIL_K7Q2MA]]] [ CPF_7H3JX9 ] [cpf_7h3jx9]",
		"end with partial [EMAIL_K7Q2",
		"bare EMAIL_K7Q2MA and (CPF 7H3JX9) and %5BIP_ABCDE%5D, EMAIL ADDRESS, THE END EMAIL_K7Q2MA",
		"\u3010EMAIL_K7Q2MA\u3011 <CPF-7H3JX9> {IP_ABCDE} [EMAIL_K7Q2MA",
	}
	for _, in := range inputs {
		for _, enc := range []Encoding{Raw, JSONString} {
			o := Options{Encoding: enc}
			want, _ := Restore(nil, []byte(in), set, o)
			for i := 0; i <= len(in); i++ {
				for j := i; j <= len(in); j++ {
					got, held := streamed([]byte(in), []int{i, j}, o)
					if !bytes.Equal(got, want) {
						t.Fatalf("enc=%d cuts=%d,%d: got %q want %q", enc, i, j, got, want)
					}
					if held >= MaxTokenLen {
						t.Fatalf("held %d >= %d", held, MaxTokenLen)
					}
				}
			}
			single, _ := streamed([]byte(in), byteCuts(len(in)), o)
			if !bytes.Equal(single, want) {
				t.Fatalf("byte-at-a-time: got %q want %q", single, want)
			}
		}
	}
}

func byteCuts(n int) []int {
	cuts := make([]int, n)
	for i := range cuts {
		cuts[i] = i
	}
	return cuts
}

func TestPropertyChunkingInvariance(t *testing.T) {
	t.Parallel()
	pieces := []string{
		"[EMAIL_K7Q2MA]",
		"[CPF_7H3JX9]",
		`\[IP\_ABCDE\]`,
		"[",
		"\\",
		"_",
		"]",
		" ",
		"abc",
		"é",
		"[EMAIL_",
		"ZZZZZZ]",
		"\n",
	}
	rapid.Check(t, func(rt *rapid.T) {
		parts := rapid.SliceOf(rapid.SampledFrom(pieces)).Draw(rt, "parts")
		src := []byte(strings.Join(parts, ""))
		n := rapid.IntRange(0, 8).Draw(rt, "n")
		cuts := make([]int, 0, n)
		for range n {
			cuts = append(cuts, rapid.IntRange(0, len(src)).Draw(rt, "cut"))
		}
		sortInts(cuts)
		want, _ := Restore(nil, src, set, Options{})
		got, held := streamed(src, cuts, Options{})
		if !bytes.Equal(got, want) {
			rt.Fatalf("got %q want %q", got, want)
		}
		if held >= MaxTokenLen {
			rt.Fatalf("held %d", held)
		}
	})
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func FuzzRestoreSplit(f *testing.F) {
	f.Add([]byte("hi [EMAIL_K7Q2MA] and \\[CPF\\_7H3JX9\\]"), uint8(3), uint8(9))
	f.Add([]byte("[[[[ ]]]] [email_zzzzzz"), uint8(1), uint8(2))
	f.Fuzz(func(t *testing.T, src []byte, a, b uint8) {
		i := min(int(a), len(src))
		j := min(max(int(b), i), len(src))
		for _, enc := range []Encoding{Raw, JSONString} {
			o := Options{Encoding: enc}
			want, _ := Restore(nil, src, set, o)
			got, held := streamed(src, []int{i, j}, o)
			if !bytes.Equal(got, want) {
				t.Fatalf("got %q want %q", got, want)
			}
			if held >= MaxTokenLen {
				t.Fatalf("held %d", held)
			}
		}
	})
}

func FuzzMatcher(f *testing.F) {
	f.Add([]byte("[EMAIL_K7Q2MA]"))
	f.Add([]byte(`\[ IBAN\_ABCDEFGHJK \]`))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, json := range []bool{true, false} {
			for _, eof := range []bool{true, false} {
				m := matchToken(b, ' ', json, eof)
				if m.res == full && (m.n > len(b) || m.n >= MaxTokenLen) {
					t.Fatalf("bad length %d", m.n)
				}
				if m.res == partial && (eof || len(b) >= MaxTokenLen) {
					t.Fatalf("partial on %d bytes (eof=%v)", len(b), eof)
				}
			}
		}
	})
}

func benchText(n int) []byte {
	unit := "Dear [EMAIL_K7Q2MA], we reviewed CPF [CPF_7H3JX9] and the address on file. Regards, team. "
	return []byte(strings.Repeat(unit, n/len(unit)+1)[:n])
}

func BenchmarkRestoreStream(b *testing.B) {
	src := benchText(64 << 10)
	for _, chunk := range []int{1, 8, 64} {
		b.Run("chunk="+strconv.Itoa(chunk), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			out := make([]byte, 0, len(src)+1024)
			for b.Loop() {
				s := NewStream(set, Options{})
				out = out[:0]
				for i := 0; i < len(src); i += chunk {
					out = s.Feed(out, src[i:min(i+chunk, len(src))])
				}
				out = s.Flush(out)
			}
		})
	}
}

func TestToolWithhold(t *testing.T) {
	src := []byte(`{"to":"[EMAIL_K7Q2MA]","id":"[CPF_7H3JX9]","n":"[CPF_N0MBER]"}`)
	o := Options{Encoding: JSONString, Tool: true, Withhold: entity.Mask(0).With(entity.CPF)}
	got, st := Restore(nil, src, set, o)
	want := `{"to":"ana@empresa.pt","id":"[CPF_7H3JX9]","n":"[CPF_N0MBER]"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if st.Restored != 1 || st.Withheld != 2 {
		t.Fatalf("stats %+v", st)
	}
	o.Tool = false
	got, st = Restore(nil, src, set, o)
	if st.Withheld != 0 || bytes.Contains(got, []byte("CPF_")) {
		t.Fatalf("non-tool channel withheld: %s %+v", got, st)
	}
	o.Tool = true
	var out []byte
	s := NewStream(set, o)
	for i := range src {
		out = s.Feed(out, src[i:i+1])
	}
	out = s.Flush(out)
	if string(out) != want {
		t.Fatalf("stream got %s", out)
	}
}

func TestTriggerMaskMatchesTable(t *testing.T) {
	t.Parallel()
	for _, json := range []bool{false, true} {
		table := newTriggers(json)
		for c := range 256 {
			for pos := range 8 {
				x := uint64(c) << (8 * pos)
				for k := range 8 {
					if k != pos {
						x |= uint64('a') << (8 * k)
					}
				}
				got := triggerMask(x, json)>>(8*pos+7)&1 == 1
				if got != table[c] {
					t.Fatalf("json=%v byte %#x at %d: mask %v table %v", json, c, pos, got, table[c])
				}
			}
		}
	}
}

func TestBareImpossibleImpliesReject(t *testing.T) {
	t.Parallel()
	alphabet := []byte("ABCDEFGHJKMNPQRSTVWXYZ0123456789IiLlOoUu_- \\[]<>(){}%\"abcxyz.,\n")
	rng := rand.New(rand.NewPCG(21, 34))
	for _, enc := range []Encoding{Raw, JSONString} {
		s := NewStream(set, Options{Encoding: enc})
		hits := 0
		for range 200000 {
			n := 1 + rng.IntN(16)
			b := make([]byte, n)
			b[0] = byte('A' + rng.IntN(26))
			for k := 1; k < n; k++ {
				b[k] = alphabet[rng.IntN(len(alphabet))]
			}
			if !s.bareImpossible(b) {
				continue
			}
			hits++
			for _, eof := range []bool{false, true} {
				if m := matchToken(b, ' ', enc == JSONString, eof); m.res != reject {
					t.Fatalf("enc=%v eof=%v %q: pre-check rejected a %v match", enc, eof, b, m.res)
				}
			}
		}
		if hits < 10000 {
			t.Fatalf("pre-check rejected only %d inputs", hits)
		}
	}
}
