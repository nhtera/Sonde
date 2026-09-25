# Benchmarks

Report-only baselines, not CI gates. Record new rows; never edit old ones.

## `sonde version` (startup + binary size)

| Date | Version | Platform | Binary size | Startup (median / mean of 200 runs) | Method |
|---|---|---|---|---|---|
| 2026-09-25 | dev (Phase 1 skeleton) | darwin/arm64, Apple M4 Pro, go1.27.1 | 2,607,186 B (`make build`, `-s -w -trimpath`) | 2.49 ms / 4.47 ms | Python `subprocess` loop (hyperfine not installed) |

Preferred method once available: `hyperfine --warmup 10 'bin/sonde version'`.
