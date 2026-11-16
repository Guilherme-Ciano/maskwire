# Contributing

## Prerequisites

- Go 1.27 (CI uses the latest 1.27.x patch release)
- golangci-lint v2.15.0
- Docker with Compose v2 for the demo

## Workflow

```bash
make lint test build
make fuzz FUZZTIME=30s
make bench
```

All changes go through a pull request with a Conventional Commits title
(`feat:`, `fix:`, `perf:`, `refactor:`, `test:`, `docs:`, `build:`, `ci:`, `chore:`).
Sign your commits off (`git commit -s`) to certify the Developer Certificate of Origin.

## No code comments

Go code in this repository contains no comments. Only compiler directives (`//go:build`, `//go:embed`)
and generated-code headers are allowed, and `go run ./internal/tools/nocomments` enforces it in CI.
Names, small functions and tests carry the intent; design rationale goes in `docs/adr/`.

## Tests that must keep passing

- `internal/restore`: streaming output equals whole-buffer output for every split point.
- `internal/mask`: mask then restore is the identity (modulo the documented surface-form rule).
- `internal/app`: end-to-end tests assert that the mock upstream never receives the raw values.

## Benchmarks

Run `make bench` on a quiet machine and compare with benchstat
(`go tool -modfile=tools/go.mod benchstat old.txt new.txt`). Only cite differences benchstat marks as significant.
Pull requests that touch Go code also get a non-blocking `benchdiff` job that benchmarks the base and the head on
the same runner and posts the benchstat table in the job summary.
