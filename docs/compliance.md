# maskwire and the GDPR, the AI Act and the LGPD

This page maps legal provisions to what maskwire does and does not do. It is engineering documentation, not legal
advice: whether a deployment complies depends on the whole processing operation, which only the controller and
its DPO can assess. maskwire is one technical measure inside that assessment.

## What maskwire is, in legal terms

maskwire **pseudonymises**: it replaces identifiers with tokens and keeps the means to reverse them (the master key
and the per-request mapping) on your side. Under the GDPR that is pseudonymisation, not anonymisation. The masked
text that reaches the provider is still personal data for you, because you can re-identify it.

## GDPR (Regulation (EU) 2016/679)

| Provision | What it says, in short | What maskwire contributes | What remains with you |
|---|---|---|---|
| Art. 4(5) | Defines pseudonymisation: data can no longer be attributed to a person without additional information that is kept separately and protected | Tokens are keyed HMACs; the key and the token-to-value mapping never leave your network, and the mapping lives only for one request | Protecting the master key, and the access controls around it |
| Recital 26 | Pseudonymised data that can be attributed with additional information is still personal data | The README and this page say so; nothing in maskwire claims anonymisation | Keeping the processing inside the GDPR: legal basis, notices, rights |
| Art. 5(1)(c) | Data minimisation | Detected identifiers are not sent to the provider; `block` keeps chosen types from leaving at all | Deciding what the prompt needs in the first place; names and free-text health data are not detected |
| Art. 25(1) | Data protection by design, naming pseudonymisation as an example measure | Deny-by-default request walking, foreign-token guard, `tool_restore` allow-lists, per-tenant policies | Choosing policies per use case and documenting that choice |
| Art. 32(1)(a) | Security of processing, naming pseudonymisation and encryption | Pseudonymisation of identifiers in transit to the provider | Encryption, TLS to the provider, host hardening, key management |
| Art. 28 | Processor contracts | Does not change the provider's role; the provider still processes personal data on your behalf | A data processing agreement with the LLM provider |
| Art. 30 | Records of processing activities | The audit log records per request which entity types were found and what was done, with no values | The record itself; the audit log is evidence for it, not a substitute |
| Art. 44-49 | Transfers to third countries | Identifiers in the prompt are replaced before the transfer, and the re-identification key stays in the EU if maskwire runs there; the EDPB treats pseudonymisation with a key kept by the exporter as a possible supplementary measure | The transfer mechanism (adequacy, SCCs) and a transfer impact assessment; context in the prompt can still identify people |
| Art. 9 | Special categories | Not detected. Free-text health, religion, ethnicity or sexual orientation pass through | Keeping special-category data out of prompts, or a separate safeguard |

Two recent sources are worth reading for the residual question of whether the provider holds personal data:

- **CJEU, C-413/23 P, EDPS v SRB, 4 September 2025.** For a recipient that receives pseudonymised data and has no
  means reasonably likely to re-identify it, the data may not be personal data *for that recipient*. It remains
  personal data for the controller that holds the key. Commentators note open questions about processors and
  Art. 28, so do not drop the DPA on the strength of this judgment.
- **EDPB Guidelines 01/2025 on pseudonymisation**, adopted 16 January 2025. They describe pseudonymisation domains,
  keeping the additional information separate, and when pseudonymisation lowers risk. maskwire's per-tenant keys
  and `token_scope: session | request` map onto the idea of limiting linkability across domains.

## AI Act (Regulation (EU) 2024/1689)

maskwire is not an AI system and does not make anyone a provider of one. It can support the logging duties of a
deployer whose own system is high-risk. The Act was amended in 2026, so check the consolidated text on EUR-Lex
for the current wording and application dates before relying on these rows.

| Provision | What it says, in short | What maskwire contributes | What remains with you |
|---|---|---|---|
| Art. 12 | High-risk systems must allow automatic recording of events (logs) over their lifetime | An HMAC-chained, per-request audit record of the LLM calls that go through it, verifiable with `maskwire audit verify --anchors` | Logging of the AI system itself: inputs, outputs and human verification as the system's own design requires |
| Art. 26(6) | Deployers keep the automatically generated logs under their control for at least six months, unless other law says otherwise | Audit files are daily JSONL per tenant; keeping them six months or more is a retention setting on your storage, and anchors detect truncation | The retention policy, and reconciling it with GDPR storage limitation |

## LGPD (Lei 13.709/2018, Brazil)

| Provision | What it says, in short | What maskwire contributes |
|---|---|---|
| Art. 12 | Anonymised data is not personal data, unless the anonymisation can be reversed | maskwire's tokens are reversible by design, so treat masked prompts as personal data |
| Art. 13 §4 | Defines pseudonymisation: the data can only be associated with a person through additional information kept separately by the controller | Same mechanism as under the GDPR: the key and mapping stay with you |
| Art. 46 | Security measures to protect personal data | Pseudonymisation of CPF, CNPJ, PIS, e-mail, phone and the other supported identifiers before they leave |

## Sources

- GDPR: https://eur-lex.europa.eu/eli/reg/2016/679/oj
- AI Act, with the consolidated version: https://eur-lex.europa.eu/eli/reg/2024/1689/oj
- CJEU C-413/23 P, EDPS v SRB: https://eur-lex.europa.eu/eli/C/2025/5551/oj/eng
- EDPB Guidelines 01/2025 on pseudonymisation: https://www.edpb.europa.eu/our-work-tools/our-documents/publication-type/guidelines_en
- LGPD: https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm
