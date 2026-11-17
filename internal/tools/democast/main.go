package main

import (
	"bytes"
	"encoding/json/v2"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	width      = 112
	lineHeight = 18
	charWidth  = 7.75
	padX       = 16
	padTop     = 44
	lineGap    = 0.28
	headerGap  = 1.4
	holdEnd    = 6.0
)

var tokenRE = regexp.MustCompile(`\[[A-Z]+_[0-9A-Z]{5,10}\]|\[(?:email|cpf|phone|iban|card)\]`)

type line struct {
	text string
	at   float64
	kind string
}

func wrap(s string) []string {
	var out []string
	r := []rune(s)
	for len(r) > width {
		cut := width
		for k := width; k > width/2; k-- {
			if r[k] == ' ' {
				cut = k
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = append([]rune("                 "), []rune(strings.TrimLeft(string(r[cut:]), " "))...)
	}
	return append(out, string(r))
}

func kindOf(s string) string {
	switch {
	case strings.HasPrefix(s, "==="):
		return "h"
	case strings.HasPrefix(s, "upstream saw"):
		return "u"
	case strings.HasPrefix(s, "client got"):
		return "g"
	case strings.HasPrefix(s, "client sent"):
		return "s"
	}
	return "o"
}

func timeline(src []byte) (out []line, total float64) {
	t := 0.6
	for raw := range strings.SplitSeq(strings.TrimRight(string(src), "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		kind := kindOf(raw)
		if kind == "h" {
			t += headerGap
		} else {
			t += lineGap
		}
		for _, w := range wrap(raw) {
			out = append(out, line{text: w, at: t, kind: kind})
		}
	}
	return out, t + holdEnd
}

func cast(lines []line) ([]byte, error) {
	var b bytes.Buffer
	h, err := json.Marshal(map[string]any{"version": 2, "width": width, "height": 40, "title": "maskwire demo"},
		json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode header: %w", err)
	}
	b.Write(append(h, '\n'))
	for _, l := range lines {
		ev, err := json.Marshal([]any{l.at, "o", l.text + "\r\n"})
		if err != nil {
			return nil, fmt.Errorf("encode event: %w", err)
		}
		b.Write(append(ev, '\n'))
	}
	return b.Bytes(), nil
}

func colored(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range tokenRE.FindAllStringIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		fmt.Fprintf(&b, `<tspan class="t">%s</tspan>`, html.EscapeString(s[m[0]:m[1]]))
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}

func svg(lines []line, total float64) []byte {
	h := padTop + len(lines)*lineHeight + 20
	w := 2*padX + int(float64(width)*charWidth)
	var b strings.Builder
	fmt.Fprintf(
		&b,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="maskwire demo: personal data masked before the upstream and restored for the client">`,
		w,
		h,
		w,
		h,
	)
	b.WriteString(
		`<style>text{font:12.5px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre;fill:#d4d4d4}`,
	)
	b.WriteString(
		`.h{fill:#7dd3fc;font-weight:600}.u{fill:#fbbf24}.g{fill:#86efac}.s{fill:#e5e5e5}.t{fill:#f59e0b;font-weight:600}`,
	)
	b.WriteString(`.g .t{fill:#f87171}`)
	for i, l := range lines {
		on := 100 * l.at / total
		fmt.Fprintf(
			&b,
			`@keyframes k%d{0%%,%.2f%%{opacity:0}%.2f%%,97%%{opacity:1}100%%{opacity:0}}.l%d{animation:k%d %.1fs linear infinite}`,
			i,
			on,
			min(on+0.4, 96.9),
			i,
			i,
			total,
		)
	}
	b.WriteString(`</style>`)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" rx="10" fill="#111827"/>`, w, h)
	b.WriteString(
		`<circle cx="22" cy="18" r="5.5" fill="#ef4444"/><circle cx="40" cy="18" r="5.5" fill="#f59e0b"/><circle cx="58" cy="18" r="5.5" fill="#22c55e"/>`,
	)
	b.WriteString(`<text x="80" y="22" style="fill:#9ca3af">$ make demo</text>`)
	for i, l := range lines {
		fmt.Fprintf(
			&b,
			`<text class="l%d %s" x="%d" y="%d">%s</text>`,
			i,
			l.kind,
			padX,
			padTop+i*lineHeight,
			colored(l.text),
		)
	}
	b.WriteString(`</svg>` + "\n")
	return []byte(b.String())
}

func run(in, castOut, svgOut string) error {
	src, err := os.ReadFile(filepath.Clean(in))
	if err != nil {
		return fmt.Errorf("read demo output: %w", err)
	}
	lines, total := timeline(src)
	c, err := cast(lines)
	if err != nil {
		return err
	}
	if err := os.WriteFile(castOut, c, 0o600); err != nil {
		return fmt.Errorf("write cast: %w", err)
	}
	if err := os.WriteFile(svgOut, svg(lines, total), 0o600); err != nil {
		return fmt.Errorf("write svg: %w", err)
	}
	return nil
}

func main() {
	in := flag.String("in", "docs/demo-output.txt", "captured output of make demo")
	castOut := flag.String("cast", "docs/demo.cast", "asciinema v2 output")
	svgOut := flag.String("svg", "docs/img/demo.svg", "animated SVG output")
	flag.Parse()
	if err := run(*in, *castOut, *svgOut); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
