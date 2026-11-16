# Benchmarks

Two kinds of numbers back the README: Go micro-benchmarks for the hot paths, and an HTTP load test that measures
the latency maskwire adds in front of an upstream.

## Micro-benchmarks

```bash
make bench
```

Runs every `Benchmark*` under `internal/` ten times with `-benchmem` and writes the output to
`bench/results/raw/<commit>.txt`. Compare two runs with benchstat and cite only differences it marks significant:

```bash
go tool -modfile=tools/go.mod benchstat bench/results/raw/old.txt bench/results/raw/new.txt
```

## Load test

`cmd/loadgen` is a separate module so its dependencies (vegeta) stay out of the proxy's `go.mod`. It starts
`mockllm` and `maskwire` from `-bin` (default `../bin`), pins them with `taskset` to `-mock-cpus` and
`-proxy-cpus` with `GOMAXPROCS=4`, and drives them at constant rates in an open model, so coordinated omission
does not hide queueing.

```bash
make build
cd bench && go run ./cmd/loadgen -rates 500,1000,2000,4000
```

For each rate it runs `-reps` repetitions of a direct call to the mock and the same call through maskwire, after a
discarded warm-up, and reports the difference at p50, p99 and p99.9. It then sends `-streams` sequential streaming
requests to each target and reports time to first byte and full-stream time. Each run writes `summary.json` and
`report.md`, with the host details, to `results/<date>-<host>-<commit>/`.

## Rules for publishing numbers

- State the hardware, kernel, Go version, CPU governor and anything else running on the host.
- Upstream latency is zero in these tests; they measure proxy overhead, not end-to-end latency.
- Keep the raw files next to every number quoted in the README or elsewhere.
