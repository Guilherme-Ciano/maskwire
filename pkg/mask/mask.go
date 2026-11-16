package mask

import (
	"errors"
	"fmt"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	imask "github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

var (
	ErrTooManyEntities = imask.ErrTooManyEntities
	ErrNoTenant        = errors.New("tenant id is required")
)

type Rule struct {
	Action   string
	MinScore float32
}

type Options struct {
	MasterKey    []byte
	KeyVersion   uint16
	Tenant       string
	Default      string
	Rules        map[string]Rule
	Regions      []string
	TokenLength  int
	MaxEntities  int
	DisablePhone bool
}

type Masker struct {
	eng    *imask.Engine
	tenant *imask.Tenant
}

type BlockedError struct {
	Types []string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("text contains blocked entity types: %v", e.Types)
}

func New(o Options) (*Masker, error) {
	if o.Tenant == "" {
		return nil, ErrNoTenant
	}
	if o.KeyVersion == 0 {
		o.KeyVersion = 1
	}
	kr, err := token.NewKeyring(o.MasterKey, o.KeyVersion)
	if err != nil {
		return nil, fmt.Errorf("keyring: %w", err)
	}
	key, err := kr.Tenant(o.Tenant)
	if err != nil {
		return nil, fmt.Errorf("tenant key: %w", err)
	}
	ct := config.Tenant{ID: o.Tenant, Default: o.Default, ByType: map[string]config.Rule{}}
	for name, r := range o.Rules {
		ct.ByType[name] = config.Rule{Action: r.Action, MinScore: r.MinScore}
	}
	p, err := ct.Policy()
	if err != nil {
		return nil, fmt.Errorf("policy: %w", err)
	}
	if o.TokenLength != 0 && (o.TokenLength < token.MinLength || o.TokenLength > token.MaxLength) {
		return nil, fmt.Errorf("token length must be in %d..%d: %w", token.MinLength, token.MaxLength, errBadOption)
	}
	det := detect.New(detect.Options{Regions: o.Regions, DisablePhone: o.DisablePhone})
	return &Masker{
		eng:    imask.NewEngine(det, o.MaxEntities),
		tenant: &imask.Tenant{ID: o.Tenant, Key: key, Policy: p, TokenLength: o.TokenLength},
	}, nil
}

var errBadOption = errors.New("invalid option")

type Session struct {
	s *imask.Session
}

func (m *Masker) Session() *Session {
	return &Session{s: m.eng.NewSession(m.tenant)}
}

func convert(err error) error {
	if b, ok := errors.AsType[*imask.BlockedError](err); ok {
		out := &BlockedError{Types: make([]string, len(b.Types))}
		for i, t := range b.Types {
			out.Types[i] = t.String()
		}
		return out
	}
	return err
}

func (s *Session) Mask(text string) (string, error) {
	out, err := s.s.MaskText(nil, []byte(text))
	if err == nil {
		err = s.s.Err()
	}
	if err != nil {
		return "", convert(err)
	}
	return string(out), nil
}

func (s *Session) MaskJSON(doc []byte) ([]byte, error) {
	out, err := s.s.MaskJSONStrings(nil, doc)
	if err == nil {
		err = s.s.Err()
	}
	if err != nil {
		return nil, convert(err)
	}
	return out, nil
}

func (s *Session) Restore(text string) string {
	out, _ := restore.Restore(nil, []byte(text), s.s, restore.Options{})
	return string(out)
}

func (s *Session) Counts() map[string]int {
	out := map[string]int{}
	for t, n := range s.s.Counters().Detected {
		if n > 0 {
			out[entity.Type(t).String()] = int(n)
		}
	}
	return out
}

func (s *Session) Close() {
	s.s.Release()
}

type Restorer struct {
	st restore.Stream
}

func (s *Session) Restorer() *Restorer {
	return &Restorer{st: restore.NewStream(s.s, restore.Options{})}
}

func (r *Restorer) Write(dst, chunk []byte) []byte {
	return r.st.Feed(dst, chunk)
}

func (r *Restorer) Flush(dst []byte) []byte {
	return r.st.Flush(dst)
}

type Entity struct {
	Type       string
	Start, End int
	Score      float32
}

func Detect(text string) []Entity {
	spans := detect.New(detect.Options{}).Detect(nil, []byte(text))
	out := make([]Entity, len(spans))
	for i, s := range spans {
		out[i] = Entity{Type: s.Type.String(), Start: s.Start, End: s.End, Score: s.Score}
	}
	return out
}
