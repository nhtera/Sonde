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

## Engine overhead (`BenchmarkRunSource` vs `BenchmarkRawHTTP`)

Both in `engine/bench_test.go`; each iteration opens one new connection to
a local server (a run of a file uses its own connections). Raw: `GET` with
a fresh `http.Transport`, body read. Engine: `Runner.RunSource` on a
one-entry file (parse, request, implicit status assert, one JSONPath
assert).

| Date | Version | Platform | Raw (ns/op, B/op, allocs) | Engine (ns/op, B/op, allocs) | Method |
|---|---|---|---|---|---|
| 2026-09-26 | dev (Phase 4) | darwin/arm64, Apple M4 Pro, go1.27.1 | 150k–174k, 23.1 kB, 152 | 130k–136k, 43 kB, 370 | `go test ./engine -run '^$' -bench . -benchmem -benchtime=200x`, 5 runs (range) |

Target: ≤ 5 ms overhead per request. The engine's overhead is below the
noise of connection setup. Keep `-benchtime` low: at thousands of
iterations macOS runs out of ephemeral ports (`TIME_WAIT`).

## Parallel runs (`BenchmarkRunAll`, `BenchmarkRunAllMemory`)

`BenchmarkRunAll` runs 100 one-request files against a local server
answering in 20 ms. `BenchmarkRunAllMemory` runs 10,000 files with no
hooks retaining results and reports the heap after a GC.

| Date | Version | Platform | 100 files, jobs=1 | 100 files, jobs=8 | Heap after 10,000 files | Method |
|---|---|---|---|---|---|---|
| 2026-09-26 | dev (Phase 5) | darwin/arm64, Apple M4 Pro, go1.27.1 | 2.15 s | 0.28 s (7.7×) | 1.1 MiB | `go test ./engine -run '^$' -bench 'BenchmarkRunAll' -benchtime=1x` |

Targets: `--jobs 8` at least 3× faster than `--jobs 1`; memory independent of
the number of files.

## Data rows (`--data`)

A one-entry file whose entry is skipped (`[Options] skip: true`, so no
connection is made) runs once per row of a generated data file with a
secret column (`--data-secret password`); peak RSS from `/usr/bin/time -l`.

| Date | Version | Platform | Rows | Plain run | `--test --jobs 8` |
|---|---|---|---|---|---|
| 2026-09-26 | dev (Phase 6) | darwin/arm64, Apple M4 Pro, go1.27.1 | 1,000 CSV | 0.03 s, 18.6 MB | 0.02 s, 20.2 MB |
| 2026-09-26 | dev (Phase 6) | darwin/arm64, Apple M4 Pro, go1.27.1 | 1,000,000 CSV (35 MB) | 31 s, 21.8 MB | 29 s, 25.3 MB |
| 2026-09-26 | dev (Phase 6) | darwin/arm64, Apple M4 Pro, go1.27.1 | 1,000,000 JSON (68 MB) | 34 s, 22.5 MB | 31 s, 24.6 MB |

Target: memory independent of the number of rows (without `--report-*`,
which keeps every result for the reports). The time is the per-unit setup
(about 30 µs a row); with real requests, each row's new connection dominates.
