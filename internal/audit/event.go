package audit

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

const (
	Version     = 1
	hashPrefix  = "sha256:"
	maxModelLen = 64
)

type Event struct {
	TS           time.Time
	Tenant       string
	Route        string
	Upstream     string
	Model        string
	Mode         string
	Status       int
	Streaming    bool
	Detected     [entity.Count]uint32
	Mask         uint32
	Redact       uint32
	Block        uint32
	Allow        uint32
	Restored     uint32
	Foreign      uint32
	Withheld     uint32
	MaskMicros   int64
	TotalMicros  int64
	InputDigest  [32]byte
	OutputDigest [32]byte
	HasOutput    bool
	Generated    [entity.Count]uint32
}

func SafeModel(s string) string {
	if len(s) > maxModelLen {
		return "invalid"
	}
	for i := range len(s) {
		c := s[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' ||
			c == '_' ||
			c == ':' ||
			c == '/'
		if !ok {
			return "invalid"
		}
	}
	return s
}

func appendDigest(dst []byte, d *[32]byte) []byte {
	dst = append(dst, '"')
	dst = append(dst, hashPrefix...)
	dst = hex.AppendEncode(dst, d[:])
	return append(dst, '"')
}

func appendKV(dst []byte, key string) []byte {
	dst = append(dst, ',', '"')
	dst = append(dst, key...)
	return append(dst, '"', ':')
}

func appendMillis(dst []byte, micros int64) []byte {
	return strconv.AppendFloat(dst, float64(micros)/1000, 'f', 3, 64)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func appendCounts(dst []byte, counts *[entity.Count]uint32) []byte {
	dst = append(dst, '{')
	first := true
	for t, n := range counts {
		if n == 0 {
			continue
		}
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = jsonx.Quote(dst, []byte(entity.Type(t).String()))
		dst = append(dst, ':')
		dst = strconv.AppendUint(dst, uint64(n), 10)
	}
	return append(dst, '}')
}

func (e *Event) appendBody(dst []byte) []byte {
	dst = append(dst, `{"v":`...)
	dst = strconv.AppendInt(dst, Version, 10)
	dst = appendKV(dst, "type")
	dst = append(dst, `"request"`...)
	dst = appendKV(dst, "id")
	dst = jsonx.Quote(dst, []byte(newID()))
	dst = appendKV(dst, "ts")
	dst = jsonx.Quote(dst, e.TS.UTC().AppendFormat(nil, time.RFC3339Nano))
	dst = appendKV(dst, "tenant")
	dst = jsonx.Quote(dst, []byte(e.Tenant))
	dst = appendKV(dst, "route")
	dst = jsonx.Quote(dst, []byte(e.Route))
	dst = appendKV(dst, "upstream")
	dst = jsonx.Quote(dst, []byte(e.Upstream))
	dst = appendKV(dst, "model")
	dst = jsonx.Quote(dst, []byte(SafeModel(e.Model)))
	dst = appendKV(dst, "mode")
	dst = jsonx.Quote(dst, []byte(e.Mode))
	dst = appendKV(dst, "status")
	dst = strconv.AppendInt(dst, int64(e.Status), 10)
	dst = appendKV(dst, "streaming")
	dst = strconv.AppendBool(dst, e.Streaming)
	dst = appendKV(dst, "detections")
	dst = appendCounts(dst, &e.Detected)
	dst = appendKV(dst, "response_detections")
	dst = appendCounts(dst, &e.Generated)
	dst = appendKV(dst, "actions")
	dst = append(dst, `{"mask":`...)
	dst = strconv.AppendUint(dst, uint64(e.Mask), 10)
	dst = append(dst, `,"redact":`...)
	dst = strconv.AppendUint(dst, uint64(e.Redact), 10)
	dst = append(dst, `,"block":`...)
	dst = strconv.AppendUint(dst, uint64(e.Block), 10)
	dst = append(dst, `,"allow":`...)
	dst = strconv.AppendUint(dst, uint64(e.Allow), 10)
	dst = append(dst, '}')
	dst = appendKV(dst, "restore")
	dst = append(dst, `{"restored":`...)
	dst = strconv.AppendUint(dst, uint64(e.Restored), 10)
	dst = append(dst, `,"foreign":`...)
	dst = strconv.AppendUint(dst, uint64(e.Foreign), 10)
	if e.Withheld > 0 {
		dst = append(dst, `,"tool_withheld":`...)
		dst = strconv.AppendUint(dst, uint64(e.Withheld), 10)
	}
	dst = append(dst, '}')
	dst = appendKV(dst, "latency_ms")
	dst = append(dst, `{"mask":`...)
	dst = appendMillis(dst, e.MaskMicros)
	dst = append(dst, `,"total":`...)
	dst = appendMillis(dst, e.TotalMicros)
	dst = append(dst, '}')
	dst = appendKV(dst, "input_digest")
	dst = appendDigest(dst, &e.InputDigest)
	if e.HasOutput {
		dst = appendKV(dst, "output_digest")
		dst = appendDigest(dst, &e.OutputDigest)
	}
	return dst
}

type Sealer struct {
	key []byte
}

func NewSealer(key []byte) *Sealer {
	return &Sealer{key: append([]byte(nil), key...)}
}

func (s *Sealer) sum(b []byte) [32]byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(b)
	var out [32]byte
	m.Sum(out[:0])
	return out
}

func (s *Sealer) Genesis(tenant, day string) [32]byte {
	return s.sum([]byte("maskwire-audit-v1|genesis|" + tenant + "|" + day))
}

func (s *Sealer) seal(dst []byte, prev *[32]byte) (line []byte, digest [32]byte) {
	dst = appendKV(dst, "prev")
	dst = appendDigest(dst, prev)
	sum := s.sum(dst)
	dst = appendKV(dst, "hash")
	dst = appendDigest(dst, &sum)
	return append(dst, '}', '\n'), sum
}

func headerBody(dst []byte, kind string, ts time.Time) []byte {
	dst = append(dst, `{"v":`...)
	dst = strconv.AppendInt(dst, Version, 10)
	dst = appendKV(dst, "type")
	dst = jsonx.Quote(dst, []byte(kind))
	dst = appendKV(dst, "ts")
	return jsonx.Quote(dst, ts.UTC().AppendFormat(nil, time.RFC3339Nano))
}

func recoveryBody(dst []byte, ts time.Time, torn int) []byte {
	dst = headerBody(dst, "recovery", ts)
	dst = appendKV(dst, "torn_bytes")
	return strconv.AppendInt(dst, int64(torn), 10)
}

func openBody(dst []byte, ts time.Time, prevDay string, prevHead *[32]byte) []byte {
	dst = headerBody(dst, "open", ts)
	dst = appendKV(dst, "prev_day")
	dst = jsonx.Quote(dst, []byte(prevDay))
	dst = appendKV(dst, "prev_day_head")
	if prevHead == nil {
		return append(dst, "null"...)
	}
	return appendDigest(dst, prevHead)
}
