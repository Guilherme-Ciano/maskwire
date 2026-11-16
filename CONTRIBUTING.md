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

## CI/CD

Every action is pinned to a full commit SHA, every job runs on `ubuntu-24.04` behind `step-security/harden-runner`,
and workflows start with read-only `contents` permission, widening it per job only where needed.

| Workflow | Trigger | What it does |
|---|---|---|
| `ci` | push to `main`, pull requests | golangci-lint, no-comments check, `go mod tidy -diff`, govulncheck; generated docs (`docs/configuration.md`, `docs/evaluation.md`) up to date; race tests with an 80% coverage gate; fuzzing of seven targets; build and benchmark smoke run; end-to-end compose demo that fails if raw personal data reaches the upstream and verifies the audit chain; multi-arch image builds. On `main` it publishes the coverage badge to the `badges` branch. |
| `codeql` | push, pull requests, weekly | CodeQL `security-and-quality` analysis for Go |
| `pr-title` | pull requests | Conventional Commits title, which release-please relies on |
| `benchdiff` | pull requests touching Go | benchstat of base vs head on the same runner; never blocks |
| `release` | push to `main` | release-please PR and tag; on release, goreleaser builds signed binaries, SBOMs and a cosign-signed multi-arch image, then build provenance is attested for the checksums and the image digest |
| `scorecard` | push, weekly | OpenSSF Scorecard, results uploaded to code scanning |
| `dependabot-auto-merge` | Dependabot PRs | auto-merges minor and patch updates once required checks pass; libphonenumber updates always wait for review because they change detection |

Repository settings the workflows expect:

- Branch protection on `main` requiring the `ci`, `codeql` and `pr-title` checks, and "Allow auto-merge".
- GitHub Pages is not needed; the coverage badge is read from the `badges` branch, which `ci` creates.
- Optional: a GitHub App with `RELEASE_APP_CLIENT_ID` (variable) and `RELEASE_APP_PRIVATE_KEY` (secret), so the
  release-please PR triggers CI. Without it the default token is used and CI must be re-run on that PR by hand.
