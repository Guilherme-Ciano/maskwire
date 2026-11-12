package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrBrokenChain = errors.New("audit chain broken")

type ChainError struct {
	File   string
	Line   int
	Reason string
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Reason)
}

func (e *ChainError) Unwrap() error {
	return ErrBrokenChain
}

const sealSuffixLen = len(`,"hash":"sha256:`) + 64 + len(`"}`)

func lineHash(line []byte) ([32]byte, bool) {
	var sum [32]byte
	if len(line) < sealSuffixLen || !bytes.HasPrefix(line[len(line)-sealSuffixLen:], []byte(`,"hash":"sha256:`)) ||
		!bytes.HasSuffix(line, []byte(`"}`)) {
		return sum, false
	}
	h := line[len(line)-sealSuffixLen+len(`,"hash":"sha256:`) : len(line)-2]
	if _, err := hex.Decode(sum[:], h); err != nil {
		return sum, false
	}
	return sum, true
}

type record struct {
	Type      string `json:"type"`
	Prev      string `json:"prev"`
	TornBytes int    `json:"torn_bytes"`
}

type Report struct {
	Files   int
	Records int
	Head    map[string]string
}

func VerifyReader(name string, r io.Reader, genesis [32]byte) (records int, head [32]byte, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	prev := genesis
	n := 0
	var pendingTorn []byte
	for sc.Scan() {
		n++
		line := sc.Bytes()
		sum, ok := lineHash(line)
		if !ok {
			if pendingTorn != nil {
				return n, prev, &ChainError{name, n - 1, "unsealed record"}
			}
			pendingTorn = append([]byte(nil), line...)
			continue
		}
		body := line[:len(line)-sealSuffixLen]
		if sha256.Sum256(body) != sum {
			return n, prev, &ChainError{name, n, "hash mismatch"}
		}
		var rec record
		if err := json.Unmarshal(append(append([]byte(nil), body...), '}'), &rec); err != nil {
			return n, prev, &ChainError{name, n, "malformed record"}
		}
		if rec.Prev != hashPrefix+hex.EncodeToString(prev[:]) {
			return n, prev, &ChainError{name, n, "prev does not match previous hash"}
		}
		if pendingTorn != nil {
			if rec.Type != "recovery" || rec.TornBytes != len(pendingTorn) {
				return n, prev, &ChainError{name, n - 1, "unsealed record"}
			}
			pendingTorn = nil
		}
		prev = sum
	}
	if err := sc.Err(); err != nil {
		return n, prev, fmt.Errorf("read %s: %w", name, err)
	}
	if pendingTorn != nil {
		return n, prev, &ChainError{name, n, "unsealed trailing record"}
	}
	return n, prev, nil
}

func VerifyFile(path string) (records int, head [32]byte, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return 0, [32]byte{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	tenant := filepath.Base(filepath.Dir(path))
	day := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return VerifyReader(path, f, Genesis(tenant, day))
}

func VerifyDir(dir string) (Report, error) {
	rep := Report{Head: map[string]string{}}
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.jsonl"))
	if err != nil {
		return rep, fmt.Errorf("list audit files: %w", err)
	}
	for _, path := range files {
		n, head, err := VerifyFile(path)
		rep.Files++
		rep.Records += n
		if err != nil {
			return rep, err
		}
		rep.Head[path] = hashPrefix + hex.EncodeToString(head[:])
	}
	return rep, nil
}
