# Changelog

## [1.1.0](https://github.com/Guilherme-Ciano/maskwire/compare/v1.0.0...v1.1.0) (2026-11-17)


### Features

* **detect:** 23 national IDs across the Americas, Asia-Pacific, Africa and the Middle East ([f10ab9e](https://github.com/Guilherme-Ciano/maskwire/commit/f10ab9e66a815bb096883c514312806b00be67d5))
* **detect:** dates of birth next to a birth keyword ([ff87492](https://github.com/Guilherme-Ciano/maskwire/commit/ff874924248c02460c04a5977b34a4befd568ec7))
* **detect:** JSON field names as detection context ([8c0348a](https://github.com/Guilherme-Ciano/maskwire/commit/8c0348a72bad34cf21c48e200aa3d7b33bb7619d))
* **detect:** mask credentials and tighten the scan loop ([344bf8e](https://github.com/Guilherme-Ciano/maskwire/commit/344bf8e65f0b7d83494c4cbd6298ad3334db8482))
* **gemini:** mask generateContent requests and restore JSON and SSE responses ([f55d6ea](https://github.com/Guilherme-Ciano/maskwire/commit/f55d6eaf7d0512ec2ab333893e006b8b7ba0ab72))


### Bug fixes

* **detect:** stop an endless loop when a secret token is followed by a dash ([51a5c11](https://github.com/Guilherme-Ciano/maskwire/commit/51a5c1142a20aaaa7066956be463a3743ab63671))


### Performance

* **anthropic:** splice restored text into content_block_delta events without decoding them into structs ([d4975e0](https://github.com/Guilherme-Ciano/maskwire/commit/d4975e04de94f177d7d501b18b22fd89b280cca8))
* **detect:** fold keyword windows through a byte table and skip keyword scans by first letter ([f2bb074](https://github.com/Guilherme-Ciano/maskwire/commit/f2bb074b01414582c0422b9e1e7b3751cd5808e8))
* **detect:** index keywords by two-byte buckets and resolve windows from one scan per run ([1706a6a](https://github.com/Guilherme-Ciano/maskwire/commit/1706a6aef99038978e2d15df427eac9649a3d83b))
* **detect:** scan plain prose eight bytes at a time and pass title-case words without tokenising ([fab500d](https://github.com/Guilherme-Ciano/maskwire/commit/fab500d38d7b2390008a8773b7a59d082fb589e6))
* **detect:** share phone verdicts across requests through a bounded hashed cache ([82c018c](https://github.com/Guilherme-Ciano/maskwire/commit/82c018c68948932828aa43cef5b27cfd091fafd6))
* **detect:** skip plain lowercase prose through a byte-class table when no group is pending ([ed115ed](https://github.com/Guilherme-Ciano/maskwire/commit/ed115eddc1109d3fb2f3cd573fd00dea8cae7036))
* **httpapi:** pool proxy copy buffers, widen upstream buffers and HTTP/2 windows, add upstream_h2c ([f6b7e8d](https://github.com/Guilherme-Ciano/maskwire/commit/f6b7e8d06a9708224e8cf2bb9fffee2631768ff0))
* **httpapi:** read request bodies into pooled, zeroed buffers sized by Content-Length ([7fbe628](https://github.com/Guilherme-Ciano/maskwire/commit/7fbe628a3f6bacc794c78c09a609a7b6cb7716c2))
* **mask:** reuse the token of a value already issued in the session without a second HMAC ([82b218f](https://github.com/Guilherme-Ciano/maskwire/commit/82b218f4321c97db4d8cadb512bcf96fcb00c97f))
* **openai:** splice restored text into SSE chunks without decoding them into structs ([7eabf63](https://github.com/Guilherme-Ciano/maskwire/commit/7eabf63c7219d4002d38c15a9dbea7e9cb89c47e))
* **restore:** canonical-token fast path and table-driven triggers ([890e808](https://github.com/Guilherme-Ciano/maskwire/commit/890e8089bbf0f3f18c77ab2254754a2bd46be8bb))
* **restore:** scan triggers eight bytes at a time, pass plain words whole and fold labels without a map ([5775e91](https://github.com/Guilherme-Ciano/maskwire/commit/5775e91eb1002d43564c94be0b2df082d0157615))


### Documentation

* add code of conduct and the missing badges ([3394ed9](https://github.com/Guilherme-Ciano/maskwire/commit/3394ed92e9b99f456a000f8bacf595060cacbbf4))
* **bench:** load test at 5,000 req/s after the performance work ([adee395](https://github.com/Guilherme-Ciano/maskwire/commit/adee39581d609389702dc6f8375ddebd4f2f1b74))
* **bench:** publish the performance roadmap results ([8e1de05](https://github.com/Guilherme-Ciano/maskwire/commit/8e1de059db95dbc274fda899f31e778c7f2b4cca))
* **bench:** refresh micro-benchmarks after the keyword index and token reuse ([d0851ec](https://github.com/Guilherme-Ciano/maskwire/commit/d0851ec7f7b5daa7ce7ec43b9b688bd6f1e94b3b))
* **bench:** refresh micro-benchmarks for 48 entity types ([8bf36e9](https://github.com/Guilherme-Ciano/maskwire/commit/8bf36e97839b21a8dc03551408a700fb175120f0))
* **bench:** refresh micro-benchmarks with the shared phone verdict cache ([8d8af10](https://github.com/Guilherme-Ciano/maskwire/commit/8d8af10369f577c8d45b9014a6441db8ecb6926b))
* **comparison:** rerun Presidio on the corpus with world IDs, secrets and dates of birth ([5fd68a8](https://github.com/Guilherme-Ciano/maskwire/commit/5fd68a80452b435832be8707b1918e7015a9523b))

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
