# Security policy

## Supported versions

Only the latest minor release receives security fixes.

## Reporting a vulnerability

Please use GitHub Private Vulnerability Reporting on this repository (Security tab, "Report a vulnerability").
Do not open a public issue for security problems.

- Acknowledgement within 72 hours.
- Coordinated disclosure within 90 days, earlier if a fix ships sooner.

## In scope

- Personal data reaching the upstream provider when the policy says it must be masked, redacted or blocked
  (detector bypasses, encoding tricks, request shapes that skip masking).
- Personal data reaching logs, metrics labels, error messages, audit records or traces.
- Restoring a value that was not issued for the current request (cross-request or cross-tenant leakage),
  including through a semantic cache.
- Tenant authentication bypass, tenant confusion, token key derivation weaknesses.
- Audit chain forgery that `maskwire audit verify` does not detect.
- Denial of service through crafted request or stream bodies.

## Out of scope

- Detection recall for entity types maskwire does not claim to detect (for example personal names in v0.1).
- Vulnerabilities in the upstream LLM provider or in semantic-cache-gateway itself.
