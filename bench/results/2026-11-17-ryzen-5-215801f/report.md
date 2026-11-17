# maskwire load test

AMD Ryzen 5 4600G with Radeon Graphics, 12 logical CPUs, 16209904 kB RAM, Linux 7.3.4-1-cachyos, governor=powersave, boost=1, go1.27.4, GOMAXPROCS 4 (maskwire, mockllm).
Pinning: maskwire 4-7, mockllm 8-11, loadgen inherited. same-host loopback, Go mock upstream with zero latency. Commit 215801f (dirty=false), 2026-11-17T17:04:03Z.

Non-streaming `POST /v1/chat/completions`, 4236-byte request with 6 PII entities, vegeta open-model constant rate, 3s warm-up discarded, 15s measured, median of 3 runs (by p99).

| rate (req/s) | direct p50 | direct p99 | maskwire p50 | maskwire p99 | added p50 | added p99 | maskwire p99.9 | success | CPU ms / 1k req |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 500 | 213 µs | 553 µs | 444 µs | 768 µs | 231 µs | 215 µs | 1660 µs | 100.00% | 365 |
| 1000 | 190 µs | 382 µs | 370 µs | 659 µs | 181 µs | 277 µs | 1199 µs | 100.00% | 321 |
| 2000 | 180 µs | 495 µs | 382 µs | 804 µs | 202 µs | 308 µs | 1439 µs | 100.00% | 294 |
| 4000 | 175 µs | 590 µs | 350 µs | 1028 µs | 175 µs | 438 µs | 2031 µs | 100.00% | 291 |
| 5000 | 188 µs | 685 µs | 369 µs | 1215 µs | 181 µs | 530 µs | 2715 µs | 100.00% | 286 |

Streaming (`stream: true`, mockllm sends 1-7 byte content deltas with no delay), sequential requests:

| target | requests | TTFB p50 | TTFB p99 | full stream p50 | full stream p99 |
|---|---:|---:|---:|---:|---:|
| direct | 300 | 188 µs | 576 µs | 7295 µs | 9579 µs |
| maskwire | 300 | 444 µs | 758 µs | 8366 µs | 10917 µs |
