package audit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetentionKeepsChainVerifiable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for d := range 5 {
		runWriter(t, dir, sampleEvents(2, "acme", fixed.Add(time.Duration(d)*24*time.Hour)))
	}
	if _, err := VerifyDir(dir, sealer(), nil); err != nil {
		t.Fatal(err)
	}
	anchorsBefore := Anchors{"acme/2026-11-11.jsonl": {"sha256:" + strings.Repeat("0", 64)}}
	w := newWriter(t, dir, Options{RetentionDays: 2, Now: func() time.Time { return fixed.Add(4 * 24 * time.Hour) }})
	w.prune()
	files, err := dayFiles(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || dayOf(files[0]) != "2026-11-13" {
		t.Fatalf("files after retention %v", files)
	}
	if _, err := VerifyDir(dir, sealer(), nil); err != nil {
		t.Fatalf("verify after retention: %v", err)
	}
	if _, err := VerifyDir(dir, sealer(), anchorsBefore); err != nil {
		t.Fatalf("anchors for expired days must be skipped: %v", err)
	}
	ledger := ledgerPath(dir, "acme")
	data, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		ledger,
		[]byte(strings.Replace(string(data), "2026-11-12", "2026-11-10", 1)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDir(dir, sealer(), nil); !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("edited ledger: %v", err)
	}
	if err := os.Remove(ledger); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDir(dir, sealer(), nil); !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("deleted ledger must expose the gap: %v", err)
	}
}

func TestCheckpointsDetectTruncation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, sampleEvents(6, "acme", fixed))
	priv, pub, err := GenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := LoadPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := LoadPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	line, err := Checkpoint(dir, sealer(), key, fixed)
	if err != nil {
		t.Fatal(err)
	}
	cps := filepath.Join(t.TempDir(), "checkpoints.jsonl")
	if err := os.WriteFile(cps, line, 0o600); err != nil {
		t.Fatal(err)
	}
	anchors, n, err := ReadCheckpoints(cps, pubKey)
	if err != nil || n != 1 || len(anchors["acme/2026-11-11.jsonl"]) != 1 {
		t.Fatalf("read checkpoints: %v %d %v", anchors, n, err)
	}
	if _, err := VerifyDir(dir, sealer(), anchors); err != nil {
		t.Fatal(err)
	}
	mutateFile(t, FilePath(dir, "acme", "2026-11-11"), func(l []string) []string { return l[:len(l)-1] })
	if _, err := VerifyDir(dir, sealer(), anchors); !errors.Is(err, ErrBrokenChain) {
		t.Fatalf("truncation after checkpoint: %v", err)
	}
	forged := strings.Replace(string(line), "sha256:", "sha256:0", 1)
	if err := os.WriteFile(cps, []byte(forged), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCheckpoints(
		cps,
		pubKey,
	); !errors.Is(err, ErrCheckpointSignature) &&
		!errors.Is(err, ErrAnchor) {
		t.Fatalf("forged checkpoint: %v", err)
	}
	_, otherPub, err := GenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := LoadPublicKey(otherPub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cps, line, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCheckpoints(cps, other); !errors.Is(err, ErrCheckpointSignature) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := LoadPublicKey(priv); !errors.Is(err, ErrKeyFormat) {
		t.Fatalf("private key as public: %v", err)
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runWriter(t, dir, append(sampleEvents(4, "acme", fixed), sampleEvents(2, "globex", fixed.Add(24*time.Hour))...))
	sum, err := Summarize(dir, sealer(), nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Tenants) != 2 || sum.Tenants[0].Requests != 4 || sum.Tenants[0].Categories["EMAIL"] != 0+1+2+3 {
		t.Fatalf("summary %+v", sum.Tenants)
	}
	if sum.Tenants[0].Recipients["mockllm:9000"] != 4 || sum.Tenants[0].Models["gpt-4.1-mini"] != 4 {
		t.Fatalf("recipients %+v", sum.Tenants[0])
	}
	md := sum.Markdown()
	for _, want := range []string{"## Tenant `acme`", "| mockllm:9000 | 4 |", "| EMAIL | 6 |", "Art. 30"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown misses %q", want)
		}
	}
	only, err := Summarize(dir, sealer(), nil, "2026-11-12", "2026-11-12")
	if err != nil || len(only.Tenants) != 1 || only.Tenants[0].Tenant != "globex" {
		t.Fatalf("date filter: %+v %v", only.Tenants, err)
	}
}
