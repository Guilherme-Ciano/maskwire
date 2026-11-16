package httpapi

import (
	"crypto/rand"
	"net/http"

	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

const maxSessionID = 256

func (s *Server) newSession(r *http.Request, t *Tenant) *mask.Session {
	ses := s.engine.NewSession(t.Mask)
	if k := s.scopedKey(r, t); k != nil {
		ses.UseKey(k)
	}
	return ses
}

func (s *Server) scopedKey(r *http.Request, t *Tenant) *token.TenantKey {
	switch t.Scope {
	case ScopeTenant:
		return nil
	case ScopeSession:
		if id := r.Header.Get(s.opts.SessionHeader); id != "" && len(id) <= maxSessionID {
			return t.Mask.Key.Derive(append([]byte("session:"), id...))
		}
	case ScopeRequest:
	}
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	return t.Mask.Key.Derive(append([]byte("request:"), nonce[:]...))
}
