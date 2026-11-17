# Changelog

## 1.0.0 (2026-11-17)


### Features

* **app:** reload tenants on SIGHUP ([a17f9e4](https://github.com/Guilherme-Ciano/maskwire/commit/a17f9e4051398154e70b48220d114c9f00d66d5d))
* **audit:** DPO report, Ed25519 checkpoints and retention ledger ([5ec3b14](https://github.com/Guilherme-Ciano/maskwire/commit/5ec3b142fa9740c40be597cdec830c4fd3e8a707))
* **audit:** external head anchors and verify --anchors ([2d1fc7a](https://github.com/Guilherme-Ciano/maskwire/commit/2d1fc7a215fb9de16069cb32da145990d10db630))
* **audit:** hash-chained JSONL audit log with verify ([5fd8bba](https://github.com/Guilherme-Ciano/maskwire/commit/5fd8bbafdf24423bad62278f1af02a0621de77a0))
* **codec:** OpenAI chat and Anthropic messages codecs for JSON and SSE ([f9d1800](https://github.com/Guilherme-Ciano/maskwire/commit/f9d1800cfe7757372c66386fc44122a78bed980f))
* **codec:** Responses API, embeddings, passthrough routes and schema masking ([90ab622](https://github.com/Guilherme-Ciano/maskwire/commit/90ab6226022f0265abd7912c225a401001a23a87))
* **demo:** containerised demo client and Grafana dashboard ([7dfad0a](https://github.com/Guilherme-Ciano/maskwire/commit/7dfad0a131b1b96d5cfd3f7387ca1ae2b9e1389b))
* **demo:** Go mock LLM upstream, demo client and compose stack ([ad2ad18](https://github.com/Guilherme-Ciano/maskwire/commit/ad2ad18772d2d66eb186966a0fa1e7130b7fe0fd))
* **demo:** working compose round trip through semantic-cache-gateway ([e9eb4a8](https://github.com/Guilherme-Ciano/maskwire/commit/e9eb4a894ab6ef658e5b2603c6fac11017e5ffc8))
* **detect:** checksum validators and allocation-free entity detector ([f47ecd2](https://github.com/Guilherme-Ciano/maskwire/commit/f47ecd235571d265ee9b4d6d854e950b93c210ef))
* **detect:** nine more national IDs, keyword-less Steuer-ID and obfuscated e-mail ([8a51472](https://github.com/Guilherme-Ciano/maskwire/commit/8a51472b9746a2ef7848c20dc1d4f7817b062d47))
* **detect:** opt-in person-name detection ([8ba02c0](https://github.com/Guilherme-Ciano/maskwire/commit/8ba02c01103c285c752517ec09fb57d2228083c8))
* **eval:** synthetic detection benchmark with published numbers ([42954ec](https://github.com/Guilherme-Ciano/maskwire/commit/42954ec6d87b6d50c182401a7659353290a98727))
* **httpapi:** per-tenant token-bucket rate limit ([bdb55de](https://github.com/Guilherme-Ciano/maskwire/commit/bdb55deadff0844de7fd1a5b3e49f6a6bf3b0c42))
* **httpapi:** send the restored-token count as a trailer on streams ([0100400](https://github.com/Guilherme-Ciano/maskwire/commit/0100400e4c3d5ed98909fcb5eb39df93c8533eb5))
* **mask:** tenant-keyed HMAC tokens, policies and per-request masking session ([b6d368b](https://github.com/Guilherme-Ciano/maskwire/commit/b6d368bebab05e4c49843fb70b85cd38e81ef69e))
* **mask:** tenant, session and request token scopes ([b4d23de](https://github.com/Guilherme-Ciano/maskwire/commit/b4d23dec2b9ed717650208d473e6da7f941019ee))
* observe personal data that the model generates in responses ([d5853fb](https://github.com/Guilherme-Ciano/maskwire/commit/d5853fb1f9c86be846b6e9a786b31831be31ff22))
* optional prompt hint asking the model to keep placeholders intact ([7cfcbb5](https://github.com/Guilherme-Ciano/maskwire/commit/7cfcbb5fb2c05455e177348a3a365d17e1ef8e9c))
* **pkg:** public pkg/mask library ([0b41bb2](https://github.com/Guilherme-Ciano/maskwire/commit/0b41bb2d6051fc2199bab7ca80e1c40137a7ea2f))
* **proxy:** reverse proxy with tenant auth, foreign-token guard and metrics ([ed5f2f6](https://github.com/Guilherme-Ciano/maskwire/commit/ed5f2f6474c2d76a92c07d21e00eda4436919a79))
* **restore:** per-tenant tool_restore allow-list for tool-call arguments ([250f585](https://github.com/Guilherme-Ciano/maskwire/commit/250f585bf8476aaccdd2894ec6609251f227012f))
* **restore:** streaming token restorer with bounded hold-back and SSE reader ([c61be8b](https://github.com/Guilherme-Ciano/maskwire/commit/c61be8b1298735f4abcb00316477e221c54a7c6f))
* **security:** enforce tenant key length and add gen-key ([6049743](https://github.com/Guilherme-Ciano/maskwire/commit/6049743ad325c19eb6f2d69a0d0bbf95b54f0420))
* **telemetry:** add maskwire_overhead_seconds and document all metrics ([819fcea](https://github.com/Guilherme-Ciano/maskwire/commit/819fcea0368e8b508e92b895d52812234331cbf8))
* **telemetry:** OpenTelemetry tracing with W3C propagation ([e9879d3](https://github.com/Guilherme-Ciano/maskwire/commit/e9879d3d967447a28fd8bb1b1a70d3be96d0eb2a))
* **vault:** encrypted per-conversation vault for cross-turn restore ([cf4d8c0](https://github.com/Guilherme-Ciano/maskwire/commit/cf4d8c009eb331ae86efb46517ca8fd863b41007))


### Bug fixes

* close masking bypasses and audit gaps found in review ([ae1983a](https://github.com/Guilherme-Ciano/maskwire/commit/ae1983aa95c0f28df7d72f93a38ab1a45cb0ecf9))
* **codec:** preserve unknown JSON members in rewritten stream events ([1a30f5a](https://github.com/Guilherme-Ciano/maskwire/commit/1a30f5a608a756edef9810e459e8d6322bf3c91e))
* **deps:** bump golang.org/x/net to v0.60.0 ([d078d8d](https://github.com/Guilherme-Ciano/maskwire/commit/d078d8d61f5191b591c5d83aad018482fa50559c))
* **detect:** require phone context and score bare IDs lower ([345e29c](https://github.com/Guilherme-Ciano/maskwire/commit/345e29c1875535edd1435290dec10857fdd509fd))
* **detect:** stop IDs from splitting tokens and phone numbers ([ad7116d](https://github.com/Guilherme-Ciano/maskwire/commit/ad7116d9990e8c4b34578bc58ad59a4850810666))
* **restore:** restore numeric tool-argument values as JSON numbers ([a0b8738](https://github.com/Guilherme-Ciano/maskwire/commit/a0b8738d5892f449c3daded99aed34692a47fa2a))
* **restore:** restore token variants that models produce ([c0175f8](https://github.com/Guilherme-Ciano/maskwire/commit/c0175f8481a38ede32305a31fe84786ab7fc9a56))


### Performance

* **codec:** forward unchanged stream events byte for byte ([70320bc](https://github.com/Guilherme-Ciano/maskwire/commit/70320bcfc485cb6779b71786bfa6cd7f352ad618))
* **detect:** cache phone validation per request ([a593dcd](https://github.com/Guilherme-Ciano/maskwire/commit/a593dcd9d271d6f316297cddcaaff8ef2932af0c))
* **detect:** skip all-lowercase words in one tight loop ([891b2cc](https://github.com/Guilherme-Ciano/maskwire/commit/891b2cc67e8d64bb8594c6d714afa349b93bda05))
* **httpapi:** accept gzip from the upstream ([3c984d0](https://github.com/Guilherme-Ciano/maskwire/commit/3c984d0f65458f6cba19cb6b42406680f0daea85))
* **httpapi:** pool masking sessions and build one reverse proxy per route ([2241ae5](https://github.com/Guilherme-Ciano/maskwire/commit/2241ae58f944bf85465bd7d02ac36c428ec56bf2))


### Documentation

* **bench:** rerun micro-benchmarks and the load test ([cdd084e](https://github.com/Guilherme-Ciano/maskwire/commit/cdd084e2bf66954c8c13e9479d20003ce85d0d36))
* **compare:** measured comparison with Microsoft Presidio ([9f64f90](https://github.com/Guilherme-Ciano/maskwire/commit/9f64f903c5e07e7c49314dc7dde3415028973820))
* **contributing:** document the Actions permission release-please needs ([d464e50](https://github.com/Guilherme-Ciano/maskwire/commit/d464e50beb88286d83464cc700922e68c8e39527))
* **demo:** animated SVG and asciinema recording of make demo ([8b8f6c4](https://github.com/Guilherme-Ciano/maskwire/commit/8b8f6c49ae5e65cba1a9f178c3d909e16e79080e))
* document detection scores, token variants and new routes ([e6cc538](https://github.com/Guilherme-Ciano/maskwire/commit/e6cc538f2f185cb5ffb6f64c2f506b73f30832b8))
* GDPR, AI Act and LGPD mapping ([2a88f3e](https://github.com/Guilherme-Ciano/maskwire/commit/2a88f3e1696b6faf77c7fe84157547a3e519519d))
* publish benchmark and load test results, LinkedIn post ([58bd107](https://github.com/Guilherme-Ciano/maskwire/commit/58bd107866ad9e08f45c0d9ba5e494db19cd1014))
* README banner and a roadmap to become the reference ([f0c3793](https://github.com/Guilherme-Ciano/maskwire/commit/f0c3793a1681b7710a85a3d69e2c43e296ae2675))
* README with limits, architecture and quickstart; semantic-cache-gateway notes ([522b0fc](https://github.com/Guilherme-Ciano/maskwire/commit/522b0fc3672cfb64d5ef7425bfad1ab2ed5412a9))

## Changelog

All notable changes to this project are documented here. Entries are generated by release-please from
Conventional Commits; the first release, 0.1.0, is created when the release pull request is merged.
