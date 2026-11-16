package openai

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/codec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestStreamGolden(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("testdata/sse/*.sse")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, in := range files {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			s := session(t)
			for _, v := range []string{"ana.souza@correio.pt", "CPF 529.982.247-25"} {
				if _, err := s.MaskText(nil, []byte(v)); err != nil {
					t.Fatal(err)
				}
			}
			got, err := io.ReadAll(NewStreamBody(io.NopCloser(bytes.NewReader(src)), s, codec.StreamOptions{}))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(got, []byte(`"Extra"`)) {
				t.Fatalf("Go field name leaked into the stream:\n%s", got)
			}
			golden := strings.TrimSuffix(in, ".sse") + ".golden"
			if *update {
				if err := os.WriteFile(golden, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output differs from %s\n got:\n%s\nwant:\n%s", golden, got, want)
			}
		})
	}
}
