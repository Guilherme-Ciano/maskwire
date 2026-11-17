# Token survival with real models

Do models copy maskwire's tokens back, so they can be restored? This test sends masked prompts to real models and
scores their answers with maskwire's own restorer.

## Method

- `go run ./internal/tools/survival gen` masks 12 multilingual support texts (replies, translations into English,
  Spanish and Portuguese, summaries, JSON extraction, a tool-call argument object, an SMS) with names enabled. Each
  is sent twice, without and with the `prompt_hint` instruction: 24 prompts in [`prompts.jsonl`](prompts.jsonl).
- Each model answered all 24 prompts in one pass as plain chat requests. It was not told that the placeholders are
  measured. Answers are in `answers-<model>.jsonl`.
- `go run ./internal/tools/survival score` restores every answer with the session that issued the tokens. Each
  token in the prompt is then counted as copied exactly, restored from a variant the matcher accepts, deformed
  beyond restore (its suffix is still visible), or omitted. Tokens in an answer that were never issued are counted
  as invented; the foreign-token guard would turn those into `[email]` and the like.

## Results (2026-11-17)

| model | prompt hint | answers | tokens | exact | restored from a variant | deformed, not restored | omitted by the model | invented tokens |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Claude Haiku 5.5 | off | 12 | 48 | 93.8% | 0.0% | 0.0% | 6.2% | 0 |
| Claude Haiku 5.5 | on | 12 | 48 | 93.8% | 0.0% | 0.0% | 6.2% | 0 |
| Claude Opus 5.5 | off | 12 | 48 | 95.8% | 0.0% | 0.0% | 4.2% | 0 |
| Claude Opus 5.5 | on | 12 | 48 | 95.8% | 0.0% | 0.0% | 4.2% | 0 |
| Claude Sonnet 5.5 | off | 12 | 48 | 89.6% | 0.0% | 0.0% | 10.4% | 0 |
| Claude Sonnet 5.5 | on | 12 | 48 | 93.8% | 0.0% | 0.0% | 6.2% | 0 |

Every token the models wrote was restored. The omitted ones are values a model reasonably left out, such as the
CPF in a polite reply or the phone number in an SMS; the scorer checked that none of them appears in a deformed
form. The hint made no difference except for Sonnet, where it kept two more tokens.

The prompts also exposed one detection gap: "Piotr Nowak" was not masked in case 10 because "Piotr" was not in the
first-name gazetteer at the time.

## Limits

- One sample per prompt, 48 tokens per model and setting: a small set, good for spotting systematic problems, not
  for fine differences between models.
- The models ran as Claude Code subagents, so they had that harness's system prompt rather than a bare API call.
- Only Claude models were tested. Other providers, small open models and high temperatures may deform tokens more;
  that is what the relaxed restore forms (lower case, other brackets, missing brackets) exist for.
