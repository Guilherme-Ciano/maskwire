# maskwire load test

AMD Ryzen 5 4600G with Radeon Graphics, 12 logical CPUs, 16209904 kB RAM, Linux 7.3.4-1-cachyos, governor=powersave, boost=1, go1.27.4, GOMAXPROCS 4 (maskwire, mockllm).
Pinning: maskwire 4-7, mockllm 8-11, loadgen inherited. same-host loopback, Go mock upstream with zero latency. Commit 0b41bb2 (dirty=true), 2026-11-16T23:36:34Z.

Non-streaming `POST /v1/chat/completions`, 4236-byte request with 6 PII entities, vegeta open-model constant rate, 3s warm-up discarded, 15s measured, median of 3 runs (by p99).

| rate (req/s) | direct p50 | direct p99 | maskwire p50 | maskwire p99 | added p50 | added p99 | maskwire p99.9 | success | CPU ms / 1k req |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 500 | 212 µs | 603 µs | 480 µs | 1030 µs | 268 µs | 427 µs | 2072 µs | 100.00% | 467 |
| 1000 | 188 µs | 346 µs | 388 µs | 842 µs | 200 µs | 497 µs | 1254 µs | 100.00% | 400 |
| 2000 | 181 µs | 450 µs | 398 µs | 973 µs | 217 µs | 523 µs | 1657 µs | 100.00% | 367 |
| 4000 | 168 µs | 647 µs | 385 µs | 2240 µs | 217 µs | 1593 µs | 7389 µs | 100.00% | 364 |

Streaming (`stream: true`, mockllm sends 1-7 byte content deltas with no delay), sequential requests:

| target | requests | TTFB p50 | TTFB p99 | full stream p50 | full stream p99 |
|---|---:|---:|---:|---:|---:|
| direct | 300 | 218 µs | 697 µs | 7705 µs | 9408 µs |
| maskwire | 300 | 509 µs | 1618 µs | 10435 µs | 13465 µs |
