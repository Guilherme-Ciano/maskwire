# 0001. Restore tokens inside the stream with a bounded hold-back

## Context

Models stream answers as SSE events of a few bytes each. A token such as `[EMAIL_FBDYD9]` often arrives split
across events. Proxies that restore values usually buffer the whole answer, which removes the reason to stream.

## Decision

Each output channel (text, each tool call's arguments, each Anthropic content block) gets its own restorer. The
restorer is a hand-written state machine that forwards every byte that cannot start a token and holds back only a
prefix that could still become one, at most `MaxTokenLen` (48) bytes. Held bytes are released as soon as the next
byte rules the token out, and at end of stream. Events that contain no token and no held bytes are forwarded
unchanged, byte for byte.

## Consequences

- Added latency per event is microseconds and does not grow with the answer length.
- Property tests split every stream at every byte offset and require the output to equal restoring the whole
  buffer at once. Any change to the matcher must keep that property.
- The token grammar must stay bounded. Relaxed forms (lower case, other brackets, missing brackets) are accepted
  only when their suffix was issued in this request, which keeps false restores out of ordinary text.
