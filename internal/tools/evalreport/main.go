package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Guilherme-Ciano/maskwire/internal/eval"
)

var errOutdated = errors.New("docs/evaluation.md is out of date, run make eval")

func run(args []string) error {
	fs := flag.NewFlagSet("evalreport", flag.ContinueOnError)
	out := fs.String("out", "docs/evaluation.md", "output file")
	seed := fs.Uint64("seed", 1, "generator seed")
	docs := fs.Int("docs", 20000, "synthetic documents")
	check := fs.Bool("check", false, "fail if the output file is not up to date")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	doc := []byte(eval.Markdown(*seed, eval.Run(*seed, *docs, []float32{0, 0.7})))
	if *check {
		cur, err := os.ReadFile(*out)
		if err != nil {
			return fmt.Errorf("read %s: %w", *out, err)
		}
		if !bytes.Equal(cur, doc) {
			return errOutdated
		}
		return nil
	}
	if err := os.WriteFile(*out, doc, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
