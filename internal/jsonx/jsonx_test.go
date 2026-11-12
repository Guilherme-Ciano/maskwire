package jsonx

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func upperStrings(dst []byte, path []Frame, raw []byte) (out []byte, handled bool, err error) {
	if len(path) == 0 || !path[len(path)-1].Is("content") || !IsString(raw) {
		return dst, false, nil
	}
	s, err := Unquote(nil, raw)
	if err != nil {
		return dst, false, err
	}
	return Quote(dst, bytes.ToUpper(s)), true, nil
}

func TestRewritePreservesUntouchedBytes(t *testing.T) {
	t.Parallel()
	r := NewRewriter()
	in := `{ "z" : 1.50e3 , "messages":[{"role":"user","content":"hi é"},{"content":["x"]}], "content":"top" }`
	want := `{ "z" : 1.50e3 , "messages":[{"role":"user","content":"HI É"},{"content":["x"]}], "content":"TOP" }`
	got, err := r.Rewrite(nil, []byte(in), upperStrings)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func pathString(path []Frame) string {
	var sb strings.Builder
	for _, f := range path {
		if f.Array {
			sb.WriteString("/" + strconv.Itoa(f.Index))
		} else {
			sb.WriteString("/" + string(f.Name))
		}
	}
	return sb.String()
}

func TestRewritePaths(t *testing.T) {
	t.Parallel()
	r := NewRewriter()
	for _, doc := range []string{
		`{"a":[{"b":"x"},"y",["z"]],"c":"w","n":12}`,
		`{"\u0061":[{"\u0062":"x"},"y",["z"]],"c":"w","n":12}`,
	} {
		var seen []string
		_, err := r.Rewrite(nil, []byte(doc), func(dst []byte, path []Frame, raw []byte) ([]byte, bool, error) {
			seen = append(seen, pathString(path)+"="+string(raw))
			return dst, false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		want := `/a/0/b="x" /a/1="y" /a/2/0="z" /c="w" /n=12`
		if strings.Join(seen, " ") != want {
			t.Fatalf("%s: got %v", doc, seen)
		}
	}
}

func TestRewriteErrors(t *testing.T) {
	t.Parallel()
	r := NewRewriter()
	for _, in := range []string{`{"a":`, `{"a":1} x`, `[1,]`, ``, strings.Repeat("[", 100) + strings.Repeat("]", 100)} {
		if _, err := r.Rewrite(nil, []byte(in), upperStrings); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func FuzzRewriteIdentity(f *testing.F) {
	f.Add([]byte(`{"messages":[{"content":"a"}]}`))
	f.Add([]byte(`[1, "x", {"k": null}]`))
	r := NewRewriter()
	f.Fuzz(func(t *testing.T, doc []byte) {
		out, err := r.Rewrite(nil, doc, func(dst []byte, _ []Frame, _ []byte) ([]byte, bool, error) {
			return dst, false, nil
		})
		if err != nil {
			return
		}
		if !bytes.Equal(out, doc) {
			t.Fatalf("identity rewrite changed bytes: %q -> %q", doc, out)
		}
	})
}

func BenchmarkRewriteLargeUnchanged(b *testing.B) {
	r := NewRewriter()
	var sb strings.Builder
	sb.WriteString(`{"messages":[`)
	for i := range 20000 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"role":"user","content":"plain text without anything sensitive in it at all"}`)
	}
	sb.WriteString(`]}`)
	doc := []byte(sb.String())
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	out := make([]byte, 0, len(doc))
	for b.Loop() {
		var err error
		out, err = r.Rewrite(out[:0], doc, func(dst []byte, _ []Frame, _ []byte) ([]byte, bool, error) {
			return dst, false, nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
