## What and why

## Checklist

- [ ] Title follows Conventional Commits (`feat:`, `fix:`, `perf:` ...)
- [ ] `make lint test` passes, including the no-comments check
- [ ] No real personal data in code, tests, fixtures or this description
- [ ] No raw values can reach logs, metric labels, errors or audit records
- [ ] Streaming changes keep the split-point property tests green
- [ ] README or `docs/configuration.md` updated for user-visible changes
- [ ] Benchmarks compared with benchstat for changes on a hot path
