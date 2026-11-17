package audit

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

const (
	hashKey       = `,"hash":"sha256:`
	sealSuffixLen = len(hashKey) + 64 + len(`"}`)
)

func lineHash(line []byte) ([32]byte, bool) {
	var sum [32]byte
	if len(line) < sealSuffixLen || !bytes.HasPrefix(line[len(line)-sealSuffixLen:], []byte(hashKey)) ||
		!bytes.HasSuffix(line, []byte(`"}`)) {
		return sum, false
	}
	if _, err := hex.Decode(sum[:], line[len(line)-sealSuffixLen+len(hashKey):len(line)-2]); err != nil {
		return sum, false
	}
	return sum, true
}

type record struct {
	Type        string  `json:"type"`
	Prev        string  `json:"prev"`
	TornBytes   int     `json:"torn_bytes"`
	PrevDay     string  `json:"prev_day"`
	PrevDayHead *string `json:"prev_day_head"`
}

type FileResult struct {
	Path    string
	Records int
	Head    [32]byte
	Hashes  map[string]bool
	Open    *record
}

type verifier struct {
	sealer  *Sealer
	name    string
	prev    [32]byte
	n       int
	torn    []byte
	hashes  map[string]bool
	opening *record
}

func (v *verifier) line(line []byte) error {
	v.n++
	sum, ok := lineHash(line)
	if !ok {
		if v.torn != nil {
			return &ChainError{v.name, v.n - 1, "unsealed record"}
		}
		v.torn = append([]byte(nil), line...)
		return nil
	}
	body := line[:len(line)-sealSuffixLen]
	if v.sealer.sum(body) != sum {
		return &ChainError{v.name, v.n, "hash mismatch (edited record or wrong key)"}
	}
	var rec record
	if err := json.Unmarshal(append(append([]byte(nil), body...), '}'), &rec); err != nil {
		return &ChainError{v.name, v.n, "malformed record"}
	}
	if rec.Prev != hashPrefix+hex.EncodeToString(v.prev[:]) {
		return &ChainError{v.name, v.n, "prev does not match previous hash"}
	}
	if v.torn != nil {
		if rec.Type != "recovery" || rec.TornBytes != len(v.torn) {
			return &ChainError{v.name, v.n - 1, "unsealed record"}
		}
		v.torn = nil
	}
	if rec.Type == "open" && v.n == 1 {
		v.opening = &rec
	}
	v.hashes[hashPrefix+hex.EncodeToString(sum[:])] = true
	v.prev = sum
	return nil
}

func VerifyReader(name string, r io.Reader, sealer *Sealer, genesis [32]byte) (FileResult, error) {
	v := &verifier{sealer: sealer, name: name, prev: genesis, hashes: map[string]bool{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if err := v.line(sc.Bytes()); err != nil {
			return FileResult{Path: name, Records: v.n}, err
		}
	}
	res := FileResult{Path: name, Records: v.n, Head: v.prev, Hashes: v.hashes, Open: v.opening}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("read %s: %w", name, err)
	}
	if v.torn != nil {
		return res, &ChainError{name, v.n, "unsealed trailing record"}
	}
	return res, nil
}

func VerifyFile(path string, sealer *Sealer) (FileResult, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return FileResult{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	tenant := filepath.Base(filepath.Dir(path))
	return VerifyReader(path, f, sealer, sealer.Genesis(tenant, dayOf(path)))
}

type Report struct {
	Files   int
	Records int
	Head    map[string]string
}

func checkLink(cur, prev *FileResult, l ledger) error {
	if cur.Open == nil {
		if prev == nil {
			return nil
		}
		return &ChainError{cur.Path, 1, "missing open record linking to the previous day"}
	}
	if prev == nil {
		if cur.Open.PrevDay != "" && !l.covers(cur.Open.PrevDay, cur.Open.PrevDayHead) {
			return &ChainError{cur.Path, 1, "previous day " + cur.Open.PrevDay + " is missing"}
		}
		return nil
	}
	if cur.Open.PrevDay != dayOf(prev.Path) {
		return &ChainError{cur.Path, 1, "previous day file " + cur.Open.PrevDay + " is missing"}
	}
	if cur.Open.PrevDayHead == nil || !prev.Hashes[*cur.Open.PrevDayHead] {
		return &ChainError{cur.Path, 1, "previous day was truncated before the linked record"}
	}
	return nil
}

func verifyTenant(
	dir, tenant string,
	sealer *Sealer,
	anchors Anchors,
	rep *Report,
	seen map[string]bool,
) (string, error) {
	files, err := dayFiles(dir, tenant)
	if err != nil {
		return "", err
	}
	l, err := readLedger(dir, tenant, sealer)
	if err != nil {
		return "", err
	}
	var prev *FileResult
	for _, path := range files {
		res, err := VerifyFile(path, sealer)
		rep.Files++
		rep.Records += res.Records
		if err != nil {
			return "", err
		}
		if err := checkLink(&res, prev, l); err != nil {
			return "", err
		}
		key := anchorKey(tenant, dayOf(path))
		if err := anchors.check(key, &res); err != nil {
			return "", err
		}
		seen[key] = true
		rep.Head[path] = hashPrefix + hex.EncodeToString(res.Head[:])
		prev = &res
	}
	return l.deletedThrough(), nil
}

func VerifyDir(dir string, sealer *Sealer, anchors Anchors) (Report, error) {
	rep := Report{Head: map[string]string{}}
	seen := map[string]bool{}
	expired := map[string]string{}
	tenants, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return rep, fmt.Errorf("list tenants: %w", err)
	}
	for _, t := range tenants {
		through, err := verifyTenant(dir, filepath.Base(t), sealer, anchors, &rep, seen)
		if err != nil {
			return rep, err
		}
		expired[filepath.Base(t)] = through
	}
	for file := range anchors {
		tenant, day := filepath.Dir(file), dayOf(file)
		if !seen[file] && (expired[tenant] == "" || day > expired[tenant]) {
			return rep, &ChainError{filepath.Join(dir, file), 0, "anchored file is missing"}
		}
	}
	return rep, nil
}
