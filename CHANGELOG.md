# Changelog

## 1.0.0 (2026-11-13)


### Features

* **audit:** hash-chained JSONL audit log with verify ([9d97782](https://github.com/Guilherme-Ciano/maskwire/commit/9d97782943ac63140d47267599b821455a8d0554))
* **codec:** OpenAI chat and Anthropic messages codecs for JSON and SSE ([b993e2c](https://github.com/Guilherme-Ciano/maskwire/commit/b993e2c1a53f88bf1172c0268069931e3a94cdb3))
* **demo:** Go mock LLM upstream, demo client and compose stack ([e66f3e1](https://github.com/Guilherme-Ciano/maskwire/commit/e66f3e124d995cea81b1579869a121368f9b1dae))
* **demo:** working compose round trip through semantic-cache-gateway ([3824307](https://github.com/Guilherme-Ciano/maskwire/commit/382430754902458c8cd24ed6860ad231e567e571))
* **detect:** checksum validators and allocation-free entity detector ([b14843b](https://github.com/Guilherme-Ciano/maskwire/commit/b14843b10dc6b5506fadd960edc5694e616d8415))
* **mask:** tenant-keyed HMAC tokens, policies and per-request masking session ([426bc1f](https://github.com/Guilherme-Ciano/maskwire/commit/426bc1f407fa8533afeb4ae9b701a33edab0f49e))
* **proxy:** reverse proxy with tenant auth, foreign-token guard and metrics ([5738675](https://github.com/Guilherme-Ciano/maskwire/commit/57386755fe0b7fbf383afee6fd8d0b6c7f0aa144))
* **restore:** streaming token restorer with bounded hold-back and SSE reader ([ac4f9c1](https://github.com/Guilherme-Ciano/maskwire/commit/ac4f9c1580fa68b5f8be14669703387c8d6d4f1f))


### Bug Fixes

* close masking bypasses and audit gaps found in review ([4f78db8](https://github.com/Guilherme-Ciano/maskwire/commit/4f78db8d69cc70da5d65ced6bbf45287a58e0b8f))
