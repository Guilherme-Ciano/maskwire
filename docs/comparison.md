# maskwire and Presidio on the same corpus

Measured on 2026-11-17 with `go run ./internal/tools/compare -docs 2000`. maskwire and Microsoft Presidio analyse
the same 2,000 synthetic documents from the detection evaluation ([docs/evaluation.md](evaluation.md)) and are
scored the same way. Presidio ran from the official image `mcr.microsoft.com/presidio-analyzer@sha256:286e3fa7f3a7426e775e8564fe1870f1ba8f999d3ab8bbb8cc46a44355d9d6e9` with its default recognizers and the `en` NLP
model, exactly as LiteLLM's Presidio guardrail uses it.

## How to read it

- **Recall** columns use the type mapping EMAIL_ADDRESS → EMAIL, PHONE_NUMBER → PHONE, IP_ADDRESS → IP,
  IBAN_CODE → IBAN, CREDIT_CARD → CREDIT_CARD, PERSON → PERSON, US_SSN, US_ITIN, IN_PAN, IN_AADHAAR, AU_TFN,
  AU_ABN and SG_NRIC_FIN to their maskwire counterparts. The default Presidio image has no recognizer enabled for the
  European, Latin American and most Asia-Pacific identifiers, secrets or dates of birth, so it scores 0% recall on them. Presidio ships optional recognizers for some
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
| `EMAIL` | 100.0% / 100.0% / 100.0% | 0.0% | 93.8% / 100.0% / 100.0% | 2.5% |
| `PHONE` | 100.0% / 100.0% / 0.0% | 29.7% | 100.0% / 100.0% / 45.5% | 13.5% |
| `IP` | 100.0% / 100.0% / 100.0% | 0.0% | 100.0% / 100.0% / 100.0% | 8.9% |
| `CPF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 44.1% |
| `CNPJ` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 76.1% |
| `PT_NIF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `ES_DNI` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `ES_NIE` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 88.2% |
| `DE_STEUER_ID` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 69.1% |
| `FR_NIR` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 68.0% |
| `IT_CF` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 68.8% |
| `IBAN` | 100.0% / 100.0% / 100.0% | 0.0% | 73.3% / 58.3% / 64.7% | 34.1% |
| `CREDIT_CARD` | 100.0% / 100.0% / 100.0% | 0.0% | 100.0% / 100.0% / 100.0% | 0.0% |
| `NL_BSN` | 100.0% / 0.0% / 0.0% | 58.6% | 0.0% / 0.0% / 0.0% | 0.0% |
| `PL_PESEL` | 100.0% / 0.0% / 0.0% | 66.7% | 0.0% / 0.0% / 0.0% | 0.0% |
| `BE_NN` | 100.0% / 100.0% / 0.0% | 36.6% | 0.0% / 0.0% / 0.0% | 58.5% |
| `AT_SVNR` | 100.0% / 0.0% / 0.0% | 61.2% | 0.0% / 0.0% / 0.0% | 8.2% |
| `SE_PNR` | 100.0% / 100.0% / 0.0% | 31.1% | 0.0% / 0.0% / 0.0% | 0.0% |
| `FI_HETU` | 100.0% / 100.0% / 78.3% | 10.9% | 0.0% / 0.0% / 0.0% | 37.0% |
| `IE_PPSN` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `UK_NINO` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 63.9% |
| `BR_PIS` | 100.0% / 100.0% / 0.0% | 46.2% | 0.0% / 0.0% / 0.0% | 38.5% |
| `PERSON` | 100.0% / 100.0% / 0.0% | 37.9% | 100.0% / 100.0% / 100.0% | 10.3% |
| `US_SSN` | 100.0% / 100.0% / 0.0% | 23.9% | 100.0% / 100.0% / 100.0% | 0.0% |
| `US_ITIN` | 100.0% / 100.0% / 0.0% | 30.6% | 100.0% / 100.0% / 100.0% | 0.0% |
| `US_EIN` | 100.0% / 0.0% / 0.0% | 55.8% | 0.0% / 0.0% / 0.0% | 37.2% |
| `US_ABA` | 100.0% / 0.0% / 0.0% | 72.7% | 0.0% / 0.0% / 0.0% | 0.0% |
| `MX_CURP` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 82.9% |
| `MX_RFC` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 97.3% |
| `AR_CUIT` | 100.0% / 100.0% / 0.0% | 47.6% | 0.0% / 0.0% / 0.0% | 35.7% |
| `CL_RUT` | 100.0% / 100.0% / 0.0% | 22.5% | 0.0% / 0.0% / 0.0% | 65.0% |
| `CO_NIT` | 100.0% / 100.0% / 0.0% | 36.8% | 0.0% / 0.0% / 0.0% | 0.0% |
| `PE_RUC` | 100.0% / 0.0% / 0.0% | 70.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `UY_CI` | 100.0% / 84.6% / 0.0% | 47.1% | 0.0% / 0.0% / 0.0% | 44.1% |
| `IN_AADHAAR` | 100.0% / 100.0% / 0.0% | 34.7% | 0.0% / 0.0% / 0.0% | 59.2% |
| `IN_PAN` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 87.2% |
| `AU_TFN` | 100.0% / 0.0% / 0.0% | 68.9% | 0.0% / 0.0% / 0.0% | 57.8% |
| `AU_ABN` | 100.0% / 94.4% / 0.0% | 22.0% | 0.0% / 0.0% / 0.0% | 58.0% |
| `NZ_IRD` | 100.0% / 0.0% / 0.0% | 65.9% | 0.0% / 0.0% / 0.0% | 11.4% |
| `JP_MYNUMBER` | 100.0% / 0.0% / 0.0% | 69.8% | 0.0% / 0.0% / 0.0% | 52.8% |
| `KR_RRN` | 100.0% / 100.0% / 0.0% | 34.4% | 0.0% / 0.0% / 0.0% | 15.6% |
| `CN_RIC` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 72.5% |
| `SG_NRIC` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 96.9% |
| `ZA_ID` | 100.0% / 0.0% / 0.0% | 63.3% | 0.0% / 0.0% / 0.0% | 0.0% |
| `IL_ID` | 100.0% / 0.0% / 0.0% | 57.4% | 0.0% / 0.0% / 0.0% | 0.0% |
| `TR_TCKN` | 100.0% / 0.0% / 0.0% | 75.0% | 0.0% / 0.0% / 0.0% | 0.0% |
| `SECRET` | 100.0% / 100.0% / 100.0% | 0.0% | 0.0% / 0.0% / 0.0% | 84.4% |
| `DOB` | 100.0% / 0.0% / 0.0% | 60.0% | 0.0% / 0.0% / 0.0% | 0.0% |

| Hard negative | maskwire flagged | Presidio flagged |
|---|---:|---:|
| CPF with a wrong check digit | 0.7% | 35.1% |
| IBAN with a wrong check digit | 0.0% | 17.7% |
| UUID | 0.0% | 47.9% |
| build number (4 dotted parts) | 0.0% | 19.2% |
| capitalized common words | 0.0% | 32.2% |
| card number failing Luhn | 0.0% | 48.7% |
| date | 0.0% | 100.0% |
| git commit hash | 0.0% | 6.9% |
| order number (5-8 digits) | 0.0% | 84.4% |
| place or institution containing a first name | 0.0% | 78.5% |
| price | 0.0% | 3.0% |
| prose after a credential word | 0.0% | 23.1% |
| quantity with unit | 0.0% | 20.7% |
| random 11 digits after a non-ID word | 0.0% | 100.0% |
| random 9 digits after a non-ID word | 9.0% | 100.0% |
| reserved-domain e-mail | 0.0% | 100.0% |
| time | 0.0% | 46.2% |
| unix timestamp | 0.0% | 100.0% |
| version string | 0.0% | 57.6% |

2000 documents, 133 KB. maskwire: 7.9 ms in process on one goroutine (17 MB/s). Presidio: 14.5 s over HTTP with 8 concurrent requests (0.01 MB/s).

Throughput is not like for like: maskwire ran in process, Presidio behind its HTTP API with spaCy's large model.
Both are what a proxy in front of an LLM would pay per request: about 4 µs per document for maskwire against
about 58 ms of Presidio service time (14.5 s × 8 workers / 2,000 documents), roughly four orders of magnitude.

## Where Presidio is better

- **Names in lowercase or unusual positions.** Its NER model found every lowercase name in the corpus, which maskwire does not
  detect at all, and it does not depend on a first-name list.
- **Phone numbers without context.** It found 45% of bare national numbers; maskwire masks a phone only with a `+`,
  parentheses or a nearby keyword, by design.
- **Bare US numbers.** It found every SSN and ITIN even with no keyword, where maskwire requires a keyword or the
  dashed layout, because a bare nine-digit number has no checksum. It also covers US passports, driver's licences
  and bank account numbers, which maskwire does not.

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
