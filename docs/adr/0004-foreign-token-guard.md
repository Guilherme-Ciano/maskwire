# 0004. Restore only tokens issued for the current request

## Context

A semantic cache in front of the provider can answer customer B with the response it stored for customer A. The
two prompts differ only in their tokens, so a naive restorer would either leak A's tokens or, with a shared vault,
A's real values.

## Decision

The restorer looks tokens up only in the current request's session. A well-formed token that this request did not
issue is replaced with a neutral label such as `[email]` (or kept as is with `foreign_token: keep`) and counted in
`maskwire_foreign_tokens_total`.

## Consequences

- Cross-subject cache hits cannot disclose another customer's data through maskwire.
- Multi-turn conversations only restore tokens that appear in the current request body; restoring tokens from
  earlier turns needs an encrypted per-session vault, which is on the roadmap.
