# maskwire and Presidio on the same corpus

Measured on 2026-11-17 with `go run ./internal/tools/compare -docs 2000`. maskwire and Microsoft Presidio analyse
the same 2,000 synthetic documents from the detection evaluation ([docs/evaluation.md](evaluation.md)) and are
scored the same way. Presidio ran from the official image `mcr.microsoft.com/presidio-analyzer@sha256:286e3fa7f3a7426e775e8564fe1870f1ba8f999d3ab8bbb8cc46a44355d9d6e9` with its default recognizers and the `en` NLP
model, exactly as LiteLLM's Presidio guardrail uses it.

## How to read it

- **Recall** columns use the type mapping EMAIL_ADDRESS → EMAIL, PHONE_NUMBER → PHONE, IP_ADDRESS → IP,
  IBAN_CODE → IBAN, CREDIT_CARD → CREDIT_CARD, PERSON → PERSON. The default Presidio image has no recognizer for the
  European and Brazilian identifiers, so it scores 0% recall on them. Presidio ships optional recognizers for some
  (ES_NIF, IT_FISCAL_CODE, PL_PESEL and others) that are not enabled in that image.
- **Leak** is the share of planted values that no detection of any type fully covered, which is what would reach the
  provider. Presidio's leak on types it does not recognise is below 100% because other entities it reports, such as
  US_BANK_NUMBER, DATE_TIME or PHONE_NUMBER, happen to cover the digits.
- **Flagged** hard negatives are text that is not personal data but would be masked. maskwire's flagged random
  numbers are the checksum collisions explained in the evaluation and disappear at `min_score: 0.7`. Presidio
  masks every date, timestamp, order number and long digit run because it reports DATE_TIME and US-specific numeric
  entities. That over-masking costs answer quality rather than privacy.

## Results

| Type | maskwire recall (labelled / formatted / bare) | maskwire leak | Presidio recall (labelled / formatted / bare) | Presidio leak |
|---|---|---:|---|---:|
| `EMAIL` | 100.0% / 100.0% / 100.0% | 0.0% | 83.3% / 90.0% / 100.0% | 8.0% |
| `PHONE` | 100.0% / 100.0% / 0.0% | 28.0% | 100.0% / 100.0% / 40.0% | 18.3% |
| `IP` | 100.0% / 100.0% / 100.0% | 0.0% | 100.0% / 100.0% / 100.0% | 20.2% |
| `CPF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 46.1% |
| `CNPJ` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 71.3% |
| `PT_NIF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `ES_DNI` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `ES_NIE` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 93.7% |
| `DE_STEUER_ID` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 59.5% |
| `FR_NIR` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 66.3% |
| `IT_CF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 89.5% |
| `IBAN` | 100.0% / 100.0% / 100.0% | 0.0% | 65.5% / 56.5% / 72.4% | 32.1% |
| `CREDIT_CARD` | 100.0% / 100.0% / 100.0% | 0.0% | 100.0% / 100.0% / 100.0% | 0.0% |
| `NL_BSN` | 100.0% / 0.0% / 0.0% | 60.5% | 0.0% / 0.0% / 0.0% | 0.0% |
| `PL_PESEL` | 100.0% / 0.0% / 0.0% | 58.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `BE_NN` | 100.0% / 100.0% / 0.0% | 30.1% | 0.0% / 0.0% / 0.0% | 54.8% |
| `AT_SVNR` | 100.0% / 0.0% / 0.0% | 54.5% | 0.0% / 0.0% / 0.0% | 13.0% |
| `SE_PNR` | 100.0% / 100.0% / 0.0% | 26.1% | 0.0% / 0.0% / 0.0% | 6.5% |
| `FI_HETU` | 100.0% / 100.0% / 62.5% | 10.5% | 0.0% / 0.0% / 0.0% | 50.0% |
| `IE_PPSN` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `UK_NINO` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 62.5% |
| `BR_PIS` | 100.0% / 100.0% / 0.0% | 25.0% | 0.0% / 0.0% / 0.0% | 51.2% |
| `PERSON` | 100.0% / 100.0% / 0.0% | 24.4% | 91.4% / 100.0% / 90.9% | 13.3% |

| Hard negative | maskwire flagged | Presidio flagged |
|---|---:|---:|
| CPF with a wrong check digit | 0.0% | 28.6% |
| IBAN with a wrong check digit | 0.0% | 13.1% |
| UUID | 0.0% | 41.8% |
| build number (4 dotted parts) | 0.0% | 22.9% |
| capitalized common words | 0.0% | 31.8% |
| card number failing Luhn | 0.0% | 59.5% |
| date | 0.0% | 100.0% |
| git commit hash | 0.0% | 3.8% |
| order number (5-8 digits) | 0.0% | 83.8% |
| place or institution containing a first name | 0.0% | 80.8% |
| price | 0.0% | 5.0% |
| quantity with unit | 0.0% | 19.4% |
| random 11 digits after a non-ID word | 1.8% | 100.0% |
| random 9 digits after a non-ID word | 6.5% | 100.0% |
| reserved-domain e-mail | 0.0% | 100.0% |
| time | 0.0% | 44.6% |
| unix timestamp | 0.0% | 100.0% |
| version string | 0.0% | 56.6% |

2000 documents, 131 KB. maskwire: 7.0 ms in process on one goroutine (19 MB/s). Presidio: 14.2 s over HTTP with 8 concurrent requests (0.01 MB/s).

Throughput is not like for like: maskwire ran in process, Presidio behind its HTTP API with spaCy's large model.
Both are what a proxy in front of an LLM would pay per request: about 3.5 µs per document for maskwire against
about 57 ms of Presidio service time (14.2 s × 8 workers / 2,000 documents), roughly four orders of magnitude.

## Where Presidio is better

- **Names in lowercase or unusual positions.** Its NER model found 91% of lowercase names, which maskwire does not
  detect at all, and it does not depend on a first-name list.
- **Phone numbers without context.** It found 40% of bare national numbers; maskwire masks a phone only with a `+`,
  parentheses or a nearby keyword, by design.
- **Breadth of US types** (SSN, ITIN, passport, driver's licence, bank numbers), which maskwire does not cover.

## Restoring tokens in streams

maskwire restores tokens inside the SSE stream with a bounded hold-back, so streaming stays streaming. LiteLLM's
Presidio guardrail can restore tokens with `output_parse_pii`. Its documentation does not state streaming
support, and [BerriAI/litellm#22821](https://github.com/BerriAI/litellm/issues/22821) reports tokens not being
restored in streamed responses on the Anthropic path. Check the current status before relying on it. Sources:
[LiteLLM PII masking with Presidio](https://docs.litellm.ai/docs/proxy/guardrails/pii_masking_v2),
[Presidio tutorial](https://docs.litellm.ai/docs/tutorials/presidio_pii_masking).

## Limits

- The corpus is maskwire's own generator, so it covers the formats maskwire was built for. A corpus of US
  documents would favour Presidio.
- Presidio was run with its default configuration. Enabling its optional recognizers and a multilingual NLP model
  would raise its recall on some European identifiers and on names, at more CPU per request.
