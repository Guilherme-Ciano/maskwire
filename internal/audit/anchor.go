package audit

import (
	"bufio"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrAnchor = errors.New("audit anchor check failed")

type anchorLine struct {
	TS   string `json:"ts"`
	File string `json:"file"`
	Head string `json:"head"`
}

type Anchors map[string][]string

func anchorKey(tenant, day string) string {
	return tenant + "/" + day + ".jsonl"
}

func (w *Writer) anchor() {
	if w.opts.AnchorPath == "" {
		return
	}
	var out []byte
	ts := w.opts.Now().UTC().Format(time.RFC3339Nano)
	for _, c := range w.chains {
		if c.prev == c.anchored {
			continue
		}
		line, err := json.Marshal(anchorLine{
			TS: ts, File: anchorKey(c.tenant, c.day), Head: hashPrefix + hex.EncodeToString(c.prev[:]),
		})
		if err != nil {
			w.fail("anchor", err)
			return
		}
		out = append(append(out, line...), '\n')
		c.anchored = c.prev
	}
	if len(out) == 0 {
		return
	}
	if err := appendSync(w.opts.AnchorPath, out); err != nil {
		w.fail("anchor", err)
	}
}

func appendSync(path string, b []byte) error {
	f, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open anchor file: %w", err)
	}
	_, werr := f.Write(b)
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return fmt.Errorf("write anchor file: %w", err)
	}
	return nil
}

func ReadAnchors(path string) (Anchors, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open anchors: %w", err)
	}
	defer f.Close()
	out := Anchors{}
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		var a anchorLine
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil || a.File == "" || a.Head == "" {
			return nil, fmt.Errorf("%w: %s:%d: malformed anchor", ErrAnchor, path, n)
		}
		out[a.File] = append(out[a.File], a.Head)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read anchors: %w", err)
	}
	return out, nil
}

func (a Anchors) check(file string, res *FileResult) error {
	for _, head := range a[file] {
		if !res.Hashes[head] {
			return &ChainError{res.Path, res.Records, "truncated: anchored head " + head + " is missing"}
		}
	}
	return nil
}
