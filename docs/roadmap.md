# Roadmap: from strong to reference

Today maskwire is the fastest way to keep Brazilian and European personal data out of OpenAI and Anthropic
requests without losing streaming. Every number below is measured in this repository:

- +0.22 ms added p50 at 2,000 req/s;
- 447 MB/s of detection per core;
- 0 tokens deformed or invented by three Claude models;
- about four orders of magnitude faster than Presidio, with 0% false alarms on dates and order numbers.

This roadmap is about becoming the reference for PII protection in front of LLMs, in any country and any context.
Every item names the metric it should move, and nothing ships without a number in
[evaluation](evaluation.md), [comparison](comparison.md) or [benchmarks](../bench/README.md).

Effort: S = days, M = one to two weeks, L = a month or more.

## 1. Cover the world, with the same rigour

The rule stays the same: use a checksum where one exists, require context where none exists, and never flag a
plain number on its own.

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 1.1 | **Americas.** US SSN, ITIN and EIN (format plus context, no checksum exists); US ABA routing (checksum) plus account context; MX CURP and RFC; AR CUIT/CUIL; CL RUT; CO NIT; PE RUC; UY CI. | Labelled recall ≥ 99.5% per type, random-number false flags ≤ 0.5% | M |
| 1.2 | **Brazil, complete.** CNH, RG (SP), título de eleitor, CNS (SUS card), Renavam, Mercosul plates, PIX keys (EVP UUIDs with context), bank branch and account with context. | Brazilian leak rate on realistic tickets ≤ 0.5% | M |
| 1.3 | **Asia-Pacific and Africa.** IN Aadhaar (Verhoeff) and PAN, AU TFN, Medicare and ABN, NZ IRD, JP My Number, KR RRN, CN resident ID (ISO 7064), SG NRIC/FIN, ZA ID, IL ID, TR TCKN. | The same targets as 1.1 | M |
| 1.4 | **Rest of the EU and EEA.** DK CPR, NO fødselsnummer, CZ/SK rodné číslo, RO CNP, HR OIB, BG EGN, GR AMKA, the Baltics, PT Cartão de Cidadão, ES CIF/NSS, IT P.IVA, FR SIREN/SIRET, EU VAT numbers. | Every EU/EEA country has at least one national ID | M |
| 1.5 | **Phones in every country.** National formats for all ITU regions. The region is inferred from tenant locale, `Accept-Language` and the countries of other entities in the same request. | Bare-phone recall from 0% to ≥ 60%, phone false flags still ≤ 0.1% | M |
| 1.6 | **A standalone "ID formats of the world" Go package.** Checksums and validators with test vectors from official sources, usable without maskwire. | Becomes a dependency of other projects | M |

## 2. Context, not just patterns

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 2.1 | **JSON key hints.** Fields named `cpf`, `email`, `telefone`, `nome` or `patient` raise the score; `order_id`, `sku` or `trace_id` lower it. Tool schemas and descriptions feed the same signal. | Bare-ID recall up without new false flags | S |
| 2.2 | **A scoring model** that combines checksum, keyword distance, layout, key hints, language and document type into one calibrated score per span, with `min_score` meaning the same thing for every type. | Precision-recall curves published per type | M |
| 2.3 | **Names v2.** Gazetteers for 50+ languages, including Slavic, Nordic, Turkish, Arabic and Persian transliterations, Indian, and CJK in native script and romanised. Surname lists, honorifics per language, all-caps and lowercase heuristics. | Labelled and in-sentence name recall ≥ 98% across languages | L |
| 2.4 | **Optional NER on the residue.** An ONNX model behind a build tag runs only on text the fast path left unmasked, within a per-request time budget. | Lowercase-name recall ≥ 90% (Presidio: 100% on this corpus) at ≤ 2 ms p99 added | L |
| 2.5 | **Conversation memory.** Names and IDs seen in earlier turns of a session (through the vault) are recognised later, even without their keyword. | Multi-turn leak rate ≤ single-turn | M |
| 2.6 | **Addresses and places.** Street patterns per country (Rua, Av., Calle, Straße, Rue plus a number), postcodes with context (CEP, ZIP+4, PLZ, UK postcode), coordinates. | New PERSONAL_ADDRESS type at ≥ 95% labelled recall | M |
| 2.7 | **Dates of birth only.** Dates next to "data de nascimento", "DOB" or "geboren", and ages. Every other date stays untouched. | Keep 0% false flags on dates while catching DOBs | S |
| 2.8 | **Special categories (GDPR Art. 9).** ICD-10/CID codes and health, religion and orientation lexicons in context, as an opt-in sensitive-context mode. | First published numbers for Art. 9 data | M |
| 2.9 | **Secrets.** API keys, JWTs, private keys and cloud credentials, by format plus entropy, so credentials never reach a model either. | Coverage of the top 50 secret formats | S |
| 2.10 | **Quasi-identifiers.** Warn about or mask combinations such as age, city and gender that single out a person even with no ID present. | A k-anonymity style risk score in the audit record | M |
| 2.11 | **Custom recognizers in config** (pattern, checksum from the library, keywords, score) and plugins in Go or WASM, hot-reloaded with SIGHUP. | Users add a type without forking | M |
| 2.12 | **Attachments.** OCR for images and PDFs through an optional sidecar, plus column-aware detection for CSV and XLSX pasted into prompts. | First multimodal numbers | L |
| 2.13 | **Response protection.** `response_scan: mask` redacts personal data the model generates itself before it reaches users or logs. | Generated-PII leak ≤ 1% | S |

## 3. Out-measure everyone, in public

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 3.1 | **Public multilingual benchmark.** Licensed open datasets plus hand-labelled realistic tickets in PT, ES, EN, DE, FR, IT, PL and NL. Per-type precision, recall, F1 and leak rate, with a CI gate per type. | First public leaderboard of PII proxies for LLMs | L |
| 3.2 | **Leaderboard runs** against Presidio (default, all recognizers, multilingual model), LLM Guard, AWS Comprehend, Google DLP and spaCy, on the same corpus, with the harness in this repo. | maskwire best-in-class F1 on EU and LatAm types; gaps published honestly | M |
| 3.3 | **Token survival across providers.** OpenAI, Gemini, Mistral, Llama and Qwen local models, temperature sweeps, long contexts. A/B test the token format to minimise deformation. | ≥ 99% of written tokens restored on every major provider | M |
| 3.4 | **Benchmarks on fixed hardware.** A self-hosted runner reruns the load test on every release, with regression alerts. | No release regresses p99 by more than 10% | S |

## 4. Faster than anyone needs

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 4.1 | A SIMD-friendly byte-class scanner and an Aho-Corasick keyword automaton | Clean text from 447 MB/s to 1 GB/s per core | M |
| 4.2 | Zero-allocation restore and pooled SSE buffers | Streaming restore from 237 MB/s to above 300 MB/s with the relaxed grammar | M |
| 4.3 | Constant-memory streaming of very large request bodies | 100 MB bodies with flat memory | M |
| 4.4 | Upstream HTTP/2 tuning and connection reuse per tenant | ≤ +0.1 ms p50 and ≤ +0.3 ms p99 added at 5,000 req/s | S |

## 5. Everywhere LLM traffic flows

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 5.1 | **Codecs.** Gemini, AWS Bedrock (Converse), Azure OpenAI, Mistral, Cohere, OpenAI-compatible local servers (Ollama, vLLM), the OpenAI Realtime API over WebSocket, batch APIs and file uploads. | Covers the top 10 LLM APIs by usage | L |
| 5.2 | **MCP traffic** through tollkeeper: mask tool arguments and results between agents and tools. | PII protection for agent tool calls | M |
| 5.3 | **Integrations.** A LiteLLM guardrail plugin, an Envoy ext_proc filter, Kong and Traefik plugins, a Kubernetes sidecar injector and a Helm chart. | Adoption without changing app code | M |
| 5.4 | **pkg/mask compiled to WASM**, with thin Python and TypeScript SDKs and a browser playground where no data leaves the page. | Try it in 10 seconds, embed it anywhere | M |

## 6. Enterprise trust

| # | Item | Metric to move | Effort |
|---|---|---|---|
| 6.1 | Master key in a KMS or HSM (AWS KMS, GCP KMS, Vault Transit), BYOK per tenant, key rotation with no downtime through dual-version tokens | Passes enterprise security review | M |
| 6.2 | Audit sinks: Postgres, S3 Object Lock anchors, SIEM export (OTLP logs, Splunk HEC), RoPA templates per EU DPA | DPO sign-off out of the box | M |
| 6.3 | Policy by data type and region: special categories only to EU endpoints; per-request consent and purpose flags | Data-residency rules enforced in the proxy | M |
| 6.4 | Admin UI: policy editor, live detection dashboard, a paste-and-see test console, tenant self-service keys | Non-engineers can operate it | L |
| 6.5 | OSS-Fuzz, an independent security audit, SLSA level 3 provenance and reproducible builds | The trust signals reference projects carry | M |

## 7. Be the reference

- A documentation site: quickstarts per stack, threat model, design docs, and the evaluation and comparison
  pages kept current by CI.
- A write-up or conference talk on restoring tokens inside SSE streams with bounded hold-back, and on why
  deterministic tokens and caches need a foreign-token guard.
- Openly maintained test vectors for national IDs and the multilingual benchmark corpus, so other tools measure
  themselves against them.
