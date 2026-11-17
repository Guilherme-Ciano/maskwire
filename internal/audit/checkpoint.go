package audit

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var (
	ErrCheckpointSignature = errors.New("checkpoint signature does not verify")
	ErrKeyFormat           = errors.New("unexpected key format")
)

type CheckpointHead struct {
	File string `json:"file"`
	Head string `json:"head"`
}

type checkpointBody struct {
	V     int              `json:"v"`
	TS    string           `json:"ts"`
	Heads []CheckpointHead `json:"heads"`
}

type checkpointLine struct {
	V     int              `json:"v"`
	TS    string           `json:"ts"`
	Heads []CheckpointHead `json:"heads"`
	Sig   string           `json:"sig"`
}

func GenerateKey(dir string) (privPath, pubPath string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("encode private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", "", fmt.Errorf("encode public key: %w", err)
	}
	privPath, pubPath = filepath.Join(dir, "audit-ed25519.key"), filepath.Join(dir, "audit-ed25519.pub")
	if err := os.WriteFile(
		privPath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}),
		0o600,
	); err != nil {
		return "", "", fmt.Errorf("write private key: %w", err)
	}
	if err := os.WriteFile(
		pubPath,
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}),
		0o600,
	); err != nil {
		return "", "", fmt.Errorf("write public key: %w", err)
	}
	return privPath, pubPath, nil
}

func readPEM(path, kind string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	block, _ := pem.Decode(b)
	if block == nil || block.Type != kind {
		return nil, fmt.Errorf("%w: %s is not a %s PEM file", ErrKeyFormat, path, kind)
	}
	return block.Bytes, nil
}

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	der, err := readPEM(path, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an Ed25519 key", ErrKeyFormat)
	}
	return priv, nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	der, err := readPEM(path, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an Ed25519 key", ErrKeyFormat)
	}
	return pub, nil
}

func signingBytes(b checkpointBody) ([]byte, error) {
	out, err := json.Marshal(b, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode checkpoint: %w", err)
	}
	return append([]byte("maskwire-checkpoint-v1\n"), out...), nil
}

func Checkpoint(dir string, sealer *Sealer, priv ed25519.PrivateKey, now time.Time) ([]byte, error) {
	rep, err := VerifyDir(dir, sealer, nil)
	if err != nil {
		return nil, err
	}
	body := checkpointBody{V: 1, TS: now.UTC().Format(time.RFC3339)}
	for path, head := range rep.Head {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil, fmt.Errorf("relative path: %w", err)
		}
		body.Heads = append(body.Heads, CheckpointHead{File: filepath.ToSlash(rel), Head: head})
	}
	slices.SortFunc(body.Heads, func(a, b CheckpointHead) int { return strings.Compare(a.File, b.File) })
	msg, err := signingBytes(body)
	if err != nil {
		return nil, err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
	line, err := json.Marshal(
		checkpointLine{V: body.V, TS: body.TS, Heads: body.Heads, Sig: sig},
		json.Deterministic(true),
	)
	if err != nil {
		return nil, fmt.Errorf("encode checkpoint: %w", err)
	}
	return append(line, '\n'), nil
}

func ReadCheckpoints(path string, pub ed25519.PublicKey) (Anchors, int, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, 0, fmt.Errorf("open checkpoints: %w", err)
	}
	defer f.Close()
	out := Anchors{}
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		n++
		var cl checkpointLine
		if err := json.Unmarshal(sc.Bytes(), &cl); err != nil {
			return nil, n, fmt.Errorf("%w: %s:%d: malformed checkpoint", ErrAnchor, path, n)
		}
		sig, err := base64.StdEncoding.DecodeString(cl.Sig)
		if err != nil {
			return nil, n, fmt.Errorf("%w: %s:%d", ErrCheckpointSignature, path, n)
		}
		msg, err := signingBytes(checkpointBody{V: cl.V, TS: cl.TS, Heads: cl.Heads})
		if err != nil {
			return nil, n, err
		}
		if !ed25519.Verify(pub, msg, sig) {
			return nil, n, fmt.Errorf("%w: %s:%d", ErrCheckpointSignature, path, n)
		}
		for _, h := range cl.Heads {
			out[h.File] = append(out[h.File], h.Head)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, n, fmt.Errorf("read checkpoints: %w", err)
	}
	return out, n, nil
}
