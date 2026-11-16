# Architecture decision records

Go code in this repository has no comments, so design rationale lives here. Each record states the context, the
decision and what it costs. Records are immutable once accepted; a later decision supersedes an earlier one.

| # | Decision | Status |
|---|---|---|
| [0001](0001-restore-inside-the-stream.md) | Restore tokens inside the stream with a bounded hold-back | Accepted |
| [0002](0002-deterministic-hmac-tokens.md) | Deterministic HMAC tokens, with opt-in session and request scopes | Accepted |
| [0003](0003-deny-by-default-request-walking.md) | Mask every JSON string leaf except an explicit skip list | Accepted |
| [0004](0004-foreign-token-guard.md) | Restore only tokens issued for the current request | Accepted |
| [0005](0005-hmac-chained-audit-log.md) | HMAC-chained JSONL audit log with external anchors | Accepted |
| [0006](0006-checksums-over-models.md) | Checksum-validated detectors instead of an NER model | Accepted |
