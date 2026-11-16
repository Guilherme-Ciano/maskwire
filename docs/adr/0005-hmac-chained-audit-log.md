# 0005. HMAC-chained JSONL audit log with external anchors

## Context

Operators need evidence of what was masked, per tenant and per day, without the log itself becoming a store of
personal data.

## Decision

Each request writes one JSON line with counts, actions, latencies and SHA-256 digests of the masked request and
response, never values or tokens. Every line carries an HMAC-SHA256 over its body and the previous line's hash, under
a key derived from the master key. Each tenant/day file opens with a record that links to the previous day's head.
A torn final line from a crash is closed by a sealed `recovery` record. Optionally the writer appends each chain's
head to an anchor file on separate storage every 5 seconds, and `audit verify --anchors` checks those heads.

## Consequences

- Editing, reordering or deleting records in the middle of a day, and deleting earlier days, is detected.
- Truncating the newest day is detected only with anchors, and only for records older than the last anchor.
- Whoever holds the master key can forge a chain; anchors on storage that key holder cannot rewrite limit that.
