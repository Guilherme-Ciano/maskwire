package restore

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

type fakeSet map[string][]byte

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
