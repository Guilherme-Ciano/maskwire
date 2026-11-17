package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/eval"
)

type presidioSpan struct {
	Type  string  `json:"entity_type"`
	Start int     `json:"start"`
	End   int     `json:"end"`
	Score float32 `json:"score"`
}

func mapType(t string) string {
	switch t {
	case "EMAIL_ADDRESS":
		return "EMAIL"
	case "PHONE_NUMBER":
		return "PHONE"
	case "IP_ADDRESS":
		return "IP"
	case "IBAN_CODE":
		return "IBAN"
	case "CREDIT_CARD":
		return "CREDIT_CARD"
	case "PERSON":
		return "PERSON"
	case "ES_NIF":
		return "ES_DNI"
	case "ES_NIE":
		return "ES_NIE"
	case "IT_FISCAL_CODE":
		return "IT_CF"
	case "PL_PESEL":
		return "PL_PESEL"
	case "FI_PERSONAL_IDENTITY_CODE":
		return "FI_HETU"
	case "UK_NINO":
		return "UK_NINO"
	}
	return "OTHER:" + t
}

func byteOffsets(text string) []int {
	out := make([]int, 0, len(text)+1)
	for i := range text {
		out = append(out, i)
	}
	return append(out, len(text))
}

func analyze(ctx context.Context, c *http.Client, url, text string) ([]eval.ExternalSpan, error) {
	body, err := json.Marshal(map[string]string{"text": text, "language": "en"})
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var spans []presidioSpan
	if err := json.Unmarshal(raw, &spans); err != nil {
		return nil, fmt.Errorf("decode %s: %w", raw, err)
	}
	offs := byteOffsets(text)
	out := make([]eval.ExternalSpan, 0, len(spans))
	for _, s := range spans {
		if s.Start < 0 || s.End >= len(offs) || s.Start >= s.End {
			continue
		}
		out = append(
			out,
			eval.ExternalSpan{Start: offs[s.Start], End: offs[s.End], Type: mapType(s.Type), Score: s.Score},
		)
	}
	return out, nil
}

func runPresidio(url string, docs []eval.ExportDoc, workers int) ([][]eval.ExternalSpan, time.Duration, error) {
	preds := make([][]eval.ExternalSpan, len(docs))
	c := &http.Client{Timeout: 30 * time.Second}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	start := time.Now()
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				p, err := analyze(context.Background(), c, url, docs[i].Text)
				mu.Lock()
				if err != nil && first == nil {
					first = err
				}
				preds[i] = p
				mu.Unlock()
			}
		})
	}
	for i := range docs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return preds, time.Since(start), first
}

func maskwireTime(docs []eval.ExportDoc) time.Duration {
	d := detect.New(detect.Options{Names: true})
	start := time.Now()
	for _, doc := range docs {
		d.Detect(nil, []byte(doc.Text))
	}
	return time.Since(start)
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}

func row(r eval.Result, typ string) (labelled, formatted, bare, leak string, ok bool) {
	for _, t := range r.Types {
		if t.Type.String() != typ {
			continue
		}
		var gold, leaks int
		for _, c := range t.ByPres {
			gold += c.Gold
			leaks += c.Leak
		}
		if gold == 0 {
			return "", "", "", "", false
		}
		return pct(t.ByPres[eval.Labelled].Hit, t.ByPres[eval.Labelled].Gold),
			pct(t.ByPres[eval.Formatted].Hit, t.ByPres[eval.Formatted].Gold),
			pct(t.ByPres[eval.Bare].Hit, t.ByPres[eval.Bare].Gold), pct(leaks, gold), true
	}
	return "", "", "", "", false
}

func report(mw, pr eval.Result, mwTime, prTime time.Duration, docs []eval.ExportDoc, workers int) string {
	var size int
	for _, d := range docs {
		size += len(d.Text)
	}
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"| Type | maskwire recall (labelled / formatted / bare) | maskwire leak | Presidio recall (labelled / formatted / bare) | Presidio leak |\n|---|---|---:|---|---:|\n",
	)
	for _, t := range mw.Types {
		ml, mf, mb, mleak, ok := row(mw, t.Type.String())
		if !ok {
			continue
		}
		pl, pf, pb, pleak, pok := row(pr, t.Type.String())
		if !pok {
			pl, pf, pb, pleak = "-", "-", "-", "-"
			for _, pt := range pr.Types {
				if pt.Type == t.Type {
					break
				}
			}
		}
		fmt.Fprintf(
			&b,
			"| `%s` | %s / %s / %s | %s | %s / %s / %s | %s |\n",
			t.Type,
			ml,
			mf,
			mb,
			mleak,
			pl,
			pf,
			pb,
			pleak,
		)
	}
	b.WriteString("\n| Hard negative | maskwire flagged | Presidio flagged |\n|---|---:|---:|\n")
	prNeg := map[string]eval.NegResult{}
	for _, n := range pr.Negatives {
		prNeg[n.Kind] = n
	}
	for _, n := range mw.Negatives {
		p := prNeg[n.Kind]
		fmt.Fprintf(&b, "| %s | %s | %s |\n", n.Kind, pct(n.Flagged, n.Seen), pct(p.Flagged, p.Seen))
	}
	fmt.Fprintf(
		&b,
		"\n%d documents, %.0f KB. maskwire: %.1f ms in process on one goroutine (%.0f MB/s). Presidio: %.1f s over HTTP with %d concurrent requests (%.2f MB/s).\n",
		len(docs),
		float64(size)/1024,
		float64(mwTime.Microseconds())/1000,
		float64(size)/mwTime.Seconds()/1e6,
		prTime.Seconds(),
		workers,
		float64(size)/prTime.Seconds()/1e6,
	)
	return b.String()
}

func main() {
	url := flag.String("presidio", "http://localhost:15002", "Presidio analyzer URL")
	n := flag.Int("docs", 2000, "documents")
	seed := flag.Uint64("seed", 1, "corpus seed")
	workers := flag.Int("workers", 8, "concurrent Presidio requests")
	flag.Parse()
	docs := eval.Export(*seed, *n)
	preds, prTime, err := runPresidio(*url, docs, *workers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mwTime := maskwireTime(docs)
	mw := eval.Run(*seed, *n, []float32{0})[0]
	pr := eval.RunExternal(*seed, *n, []float32{0}, preds)[0]
	fmt.Print(report(mw, pr, mwTime, prTime, docs, *workers))
}
