# 0002. Deterministic HMAC tokens, with opt-in session and request scopes

## Context

Random tokens make every prompt unique, so provider prompt caches and semantic caches never hit. Plain hashes of
the value can be reversed by enumerating small identifier spaces such as CPFs.

## Decision

A token suffix is HMAC-SHA256 over the entity type and the canonical value, keyed per tenant with a key derived by
HKDF from the master key and the key version, truncated to 5-10 Crockford base32 characters. The same value in the
same tenant always becomes the same token. Tenants that value unlinkability over cache hits set `token_scope` to
`session` (key derived from the `X-Maskwire-Session` header) or `request` (fresh random key per request).

## Consequences

- Without the master key a token cannot be reversed by enumeration.
- With the default scope, the provider can tell that two requests mention the same person, though not who. The
  README states this, and `token_scope` exists for tenants who cannot accept it.
- Collisions within a request are resolved by lengthening the suffix; the suffix space is checked per request.
