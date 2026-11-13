# maskwire load test

AMD Ryzen 5 4600G with Radeon Graphics, 12 logical CPUs, 16210004 kB RAM, Linux 7.3.3-1-cachyos, governor=powersave, boost=1, go1.27.4, GOMAXPROCS 4 (maskwire, mockllm).
Pinning: maskwire 4-7, mockllm 8-11, loadgen inherited. same-host loopback, Go mock upstream with zero latency. Commit bfa5642 (dirty=true), 2026-11-12T16:59:04Z.

Non-streaming `POST /v1/chat/completions`, 4236-byte request with 6 PII entities, vegeta open-model constant rate, 3s warm-up discarded, 15s measured, median of 3 runs (by p99).

| rate (req/s) | direct p50 | direct p99 | maskwire p50 | maskwire p99 | added p50 | added p99 | maskwire p99.9 | success | CPU ms / 1k req |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 500 | 261 µs | 558 µs | 626 µs | 2142 µs | 365 µs | 1585 µs | 4363 µs | 100.00% | 544 |
| 1000 | 173 µs | 421 µs | 468 µs | 1126 µs | 294 µs | 706 µs | 1911 µs | 100.00% | 437 |
| 2000 | 179 µs | 462 µs | 487 µs | 1562 µs | 308 µs | 1100 µs | 5951 µs | 100.00% | 431 |
| 4000 | 186 µs | 815 µs | 465 µs | 2084 µs | 279 µs | 1270 µs | 8224 µs | 100.00% | 393 |

Streaming (`stream: true`, mockllm sends 1-7 byte content deltas with no delay), sequential requests:

| target | requests | TTFB p50 | TTFB p99 | full stream p50 | full stream p99 |
|---|---:|---:|---:|---:|---:|
| direct | 300 | 241 µs | 875 µs | 8055 µs | 9583 µs |
| maskwire | 300 | 567 µs | 1261 µs | 11836 µs | 17089 µs |
