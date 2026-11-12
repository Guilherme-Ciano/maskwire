package jsonx

import (
	"bytes"
	"strings"
	"testing"
)

func upperStrings(dst []byte, path []Frame, raw []byte) (out []byte, handled bool, err error) {
	if len(path) == 0 || !path[len(path)-1].Is("content") {
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

func TestRewritePaths(t *testing.T) {
	t.Parallel()
	r := NewRewriter()
	var seen []string
	_, err := r.Rewrite(
		nil,
		[]byte(`{"a":[{"b":"x"},"y",["z"]],"c":"w"}`),
		func(dst []byte, path []Frame, raw []byte) ([]byte, bool, error) {
			var sb strings.Builder
			for _, f := range path {
				if f.Array {
					sb.WriteString("/" + string(rune('0'+f.Index)))
				} else {
					sb.WriteString("/" + string(f.Name[1:len(f.Name)-1]))
				}
			}
			seen = append(seen, sb.String()+"="+string(raw))
			return dst, false, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `/a/0/b="x" /a/1="y" /a/2/0="z" /c="w"`
	if strings.Join(seen, " ") != want {
		t.Fatalf("got %v", seen)
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
