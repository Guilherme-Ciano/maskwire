# 0003. Mask every JSON string leaf except an explicit skip list

## Context

LLM request schemas change often: new message part types, tool formats, Responses API items. An allow-list of
fields to mask silently misses personal data in every new field.

## Decision

The request walker visits every string and number leaf and masks it, except a short skip list: model names, roles,
ids, tool and function names, signed thinking blocks and base64 payloads. Inside JSON Schemas only free-text
keywords (`description`, `title`, `default`, `const`, `enum`, `examples`) are masked. Key names are compared after
decoding JSON escapes.

## Consequences

- A new provider field is masked by default; the failure mode is a masked value the model did not need, not a
  leak.
- Skip-list entries need a test that shows why they are safe to forward.
