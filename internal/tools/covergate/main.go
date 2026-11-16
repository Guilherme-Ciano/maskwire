package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errBelow = errors.New("coverage below minimum")

type block struct {
	stmts   int
	covered bool
}

func parse(r io.Reader) (covered, total int, err error) {
	blocks := map[string]block{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, 0, fmt.Errorf("malformed profile line %q", line)
		}
		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])
		if err := errors.Join(err1, err2); err != nil {
			return 0, 0, fmt.Errorf("malformed profile line %q: %w", line, err)
		}
		b := blocks[fields[0]]
		b.stmts = stmts
		b.covered = b.covered || count > 0
		blocks[fields[0]] = b
	}
	if err := sc.Err(); err != nil {
		return 0, 0, fmt.Errorf("read profile: %w", err)
	}
	for _, b := range blocks {
		total += b.stmts
		if b.covered {
			covered += b.stmts
		}
	}
	return covered, total, nil
}

func gate(r io.Reader, w io.Writer, minPct float64) error {
	covered, total, err := parse(r)
	if err != nil {
		return err
	}
	if total == 0 {
		return errors.New("profile has no statements")
	}
	pct := 100 * float64(covered) / float64(total)
	if _, err := fmt.Fprintf(
		w,
		"coverage: %.1f%% of %d statements (minimum %.1f%%)\n",
		pct,
		total,
		minPct,
	); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if pct < minPct {
		return fmt.Errorf("%w: %.1f%% < %.1f%%", errBelow, pct, minPct)
	}
	return nil
}

func main() {
	profile := flag.String("profile", "cover.out", "coverage profile")
	minPct := flag.Float64("min", 80, "minimum statement coverage in percent")
	flag.Parse()
	f, err := os.Open(filepath.Clean(*profile))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	err = gate(f, os.Stdout, *minPct)
	_ = f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
