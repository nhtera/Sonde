# Benchmarks

Report-only baselines, not CI gates. Record new rows; never edit old ones.

## `sonde version` (startup + binary size)

| Date | Version | Platform | Binary size | Startup (median / mean of 200 runs) | Method |
|---|---|---|---|---|---|
| 2026-09-25 | dev (Phase 1 skeleton) | darwin/arm64, Apple M4 Pro, go1.27.1 | 2,607,186 B (`make build`, `-s -w -trimpath`) | 2.49 ms / 4.47 ms | Python `subprocess` loop (hyperfine not installed) |

Preferred method once available: `hyperfine --warmup 10 'bin/sonde version'`.

## Parse (`BenchmarkParse`, 1 MiB synthetic file)

| Date | Version | Platform | Time / MiB | Allocations | Method |
|---|---|---|---|---|---|
| 2026-09-25 | dev (Phase 2) | darwin/arm64, Apple M4 Pro, go1.27.1 | 30.8 ms | 637k allocs, 42.6 MB | `go test ./internal/syntax -bench BenchmarkParse -benchtime 30x -count 3` (median) |

Target: < 50 ms per MiB. Parse time stays linear but depends on content:
files dominated by blank and comment lines between entries measured about
165 ms/MiB (2026-09-25 review), because those lines are re-read while
optional parts of the previous entry are tried.
