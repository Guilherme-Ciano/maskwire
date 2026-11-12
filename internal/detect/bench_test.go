package detect

import (
	"strings"
	"testing"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
)

const cleanSample = "Hello team, could you summarise the meeting notes from Tuesday? We discussed the roadmap for Q3, " +
	"the migration to the new billing system and the hiring plan for the platform group. Please keep it short and " +
	"focus on decisions, owners and deadlines. Thanks a lot, and remember the retro is moved to Thursday afternoon. "

const denseSample = "Cliente Ana, CPF 529.982.247-25, email ana.silva@empresa.com.br, tel +55 11 98765-4321, " +
	"IBAN PT50 0002 0123 1234 5678 9015 4, cartão 4242 4242 4242 4242, ip 8.8.8.8. Please draft a polite reply. "

func sized(base string, n int) []byte {
	return []byte(strings.Repeat(base, n/len(base)+1)[:n])
}

func BenchmarkDetect(b *testing.B) {
	d := New(Options{})
	for _, kind := range []struct {
		name string
		base string
	}{{"clean", cleanSample}, {"dense", denseSample}} {
		for _, size := range []struct {
			name string
			n    int
		}{{"1KB", 1 << 10}, {"4KB", 4 << 10}, {"64KB", 64 << 10}} {
			text := sized(kind.base, size.n)
			b.Run(kind.name+"/"+size.name, func(b *testing.B) {
				spans := make([]entity.Span, 0, 1024)
				b.SetBytes(int64(len(text)))
				b.ReportAllocs()
				for b.Loop() {
					spans = d.Detect(spans, text)
				}
			})
		}
	}
}
