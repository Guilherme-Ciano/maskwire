package httpapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/restore"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
	"github.com/Guilherme-Ciano/maskwire/internal/vault"
)

const (
	maxSessionID = 256
	vaultSaveMax = 250 * time.Millisecond
)

func (s *Server) newSession(r *http.Request, t *Tenant) (*mask.Session, *vault.Scope) {
	ses := s.engine.NewSession(t.Mask)
	k, session := s.scopedKey(r, t)
	if k != nil {
		ses.UseKey(k)
	}
	if !session || !t.CrossTurn || s.opts.Vault == nil {
		return ses, nil
	}
	sc, err := s.opts.Vault.Open(k)
	if err != nil {
		s.log.ErrorContext(r.Context(), "vault open failed", slog.String("error", err.Error()))
		return ses, nil
	}
	return ses, sc
}

func (s *Server) scopedKey(r *http.Request, t *Tenant) (*token.TenantKey, bool) {
	switch t.Scope {
	case ScopeTenant:
		return nil, false
	case ScopeSession:
		if id := r.Header.Get(s.opts.SessionHeader); id != "" && len(id) <= maxSessionID {
			return t.Mask.Key.Derive(append([]byte("session:"), id...)), true
		}
	case ScopeRequest:
	}
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	return t.Mask.Key.Derive(append([]byte("request:"), nonce[:]...)), false
}

func (s *Server) saveVault(ctx context.Context, x *exchange) {
	if x.vault == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, vaultSaveMax)
	defer cancel()
	if err := x.vault.Save(ctx, x.session); err != nil {
		s.metrics.VaultErrors.Inc()
		s.log.WarnContext(ctx, "vault save failed; earlier-turn tokens of this request will not restore later",
			slog.String("tenant", x.tenant.Mask.ID), slog.String("error", err.Error()))
	}
}

func (x *exchange) restoreSet() restore.Set {
	if x.vault == nil {
		return x.session
	}
	return vault.Set{Session: x.session, Scope: x.vault}
}
