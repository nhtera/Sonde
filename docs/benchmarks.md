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

## CLI comparison (`scripts/bench.sh`)

`scripts/bench.sh` builds `sonde`, starts a local HTTP server on an
ephemeral port (`scripts/bench/server.go`; never 8000-8003, reserved for
the conformance harness), and measures, for each of `sonde`, `hurl`, `xh`,
`newman` and `bru` that is installed: startup (a bare version/help
invocation), a single GET request against the local server, and that
request's peak RSS. Every command is run through a shell (`sh -c`, matching
hyperfine's own default and how someone would actually type it), 10
warm-up runs then a timed batch (default 50, `SONDE_BENCH_RUNS`); median
and mean are reported in milliseconds. [hyperfine](https://github.com/sharkdp/hyperfine)
is used when installed; otherwise a `python3` fallback loop times each run
with `time.perf_counter()` in one long-lived process (so the timestamp
itself never adds a second process spawn to a sub-20ms command). Peak RSS
is `/usr/bin/time -l` on macOS, `-v` on Linux. `hurl`, `xh`, `newman` and
`bru` are skipped with a note when not installed — see `scripts/bench.sh`
usage comment for env vars.

| Date | Tool | Version | Platform | Startup median/mean | Single request median/mean | Peak RSS | Method |
|---|---|---|---|---|---|---|---|
| 2026-09-26 | sonde | dev (Phase 10) | darwin/arm64, Apple M4 Pro, go1.27.1 | 9.90 ms / 9.96 ms | 10.92 ms / 10.98 ms | 17.7 MB | `scripts/bench.sh`, `SONDE_BENCH_WARMUP=10 SONDE_BENCH_RUNS=50`, python3 fallback (hyperfine not installed) |
| 2026-09-27 | sonde | 1.0.0 (`5af437d`, no code change since the tag) | linux/amd64, GitHub `ubuntu-latest` (AMD EPYC 9V74) | 6.56 ms / 6.63 ms | 8.65 ms / 8.59 ms | 19.2 MB | `bench.yml` run 36335047021, hyperfine 1.20.0, 10 warm-up + 50 runs |
| 2026-09-27 | hurl | 8.0.1 (libcurl 8.5.0) | linux/amd64, GitHub `ubuntu-latest` (AMD EPYC 9V74) | 7.19 ms / 7.21 ms | 8.55 ms / 8.53 ms | 17.2 MB | `bench.yml` run 36335047021, hyperfine 1.20.0, 10 warm-up + 50 runs |
| 2026-09-27 | xh | 0.26.2 | linux/amd64, GitHub `ubuntu-latest` (AMD EPYC 9V74) | 1.17 ms / 1.17 ms | 10.87 ms / 10.87 ms | 8.9 MB | `bench.yml` run 36335047021, hyperfine 1.20.0, 10 warm-up + 50 runs |
| 2026-09-27 | newman | 6.2.2 (Node 22) | linux/amd64, GitHub `ubuntu-latest` (AMD EPYC 9V74) | 475.09 ms / 475.03 ms | 679.40 ms / 679.64 ms | 142.8 MB | `bench.yml` run 36335047021, hyperfine 1.20.0, 10 warm-up + 50 runs |
| 2026-09-27 | bru | 4.2.0 (Node 22) | linux/amd64, GitHub `ubuntu-latest` (AMD EPYC 9V74) | 1,246.94 ms / 1,245.26 ms | 1,374.62 ms / 1,372.52 ms | 230.8 MB | `bench.yml` run 36335047021, hyperfine 1.20.0, 10 warm-up + 50 runs |

The single request is `sonde --test` and `hurl --test` on the same
one-entry file, `xh GET` (one request, no file), `newman run` on a
one-request collection and `bru run` on a one-request collection. The
`bench` workflow (`.github/workflows/bench.yml`) installs every tool at a
pinned version and runs this weekly and on demand; its job summary has the
table. Compare rows within one run: the 2026-09-26 row is a different
machine and method. Since 2026-09-27 the `sonde` request runs in `--test`
mode (before: `sonde run`).

On this run Sonde and Hurl are close: Hurl starts slightly slower, and
their single-request medians are 0.1 ms apart. `xh`, which reads no file,
starts fastest. `newman` and `bru` start a Node.js runtime and are one to
two orders of magnitude slower with 7–12× the memory.

## LSP diagnostics (`BenchmarkDiagnostics`)

`internal/lsp.BenchmarkDiagnostics` runs full diagnostics (parse +
semantic checks) on a synthetic well-formed file of at least 1,000 lines /
40 KB — the same file `TestDiagnosticsOnLargeFile` asserts produces zero
diagnostics. Previously this was `TestLargeFileDiagnosticsBudget`, a test
that asserted a 50ms budget directly; it never ran in CI (skipped under
both `-short` and `-race`, and `ci.yml` runs neither a plain `-short`-free,
non-race `go test ./internal/lsp` in isolation). As a benchmark it always
runs, and the budget is enforced by comparing against this row instead of
a hard assertion.

| Date | Version | Platform | ns/op | B/op | allocs/op | Method |
|---|---|---|---|---|---|---|
| 2026-09-26 | dev (Phase 10) | darwin/arm64, Apple M4 Pro, go1.27.1 | 1,794,975 (~1.8 ms) | 2,502,945 | 32,292 | `go test ./internal/lsp -run '^$' -bench BenchmarkDiagnostics -benchtime=20x -benchmem` |

Target: well under the old 50ms budget (about 28× margin here).
