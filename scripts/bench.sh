#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Benchmarks `sonde` against comparable HTTP file/collection runners: hurl,
# xh, newman (Postman) and bru (Bruno), whichever are installed. Measures
# startup time (a bare version/help invocation), a single GET request
# against a local server, and that single request's peak RSS. Never binds
# 8000-8003 (reserved for the conformance harness) — the bench server binds
# an ephemeral port.
#
# Usage: scripts/bench.sh [OUTPUT.md]
#   With an argument, the markdown table is also written to that file (in
#   addition to stdout); without one, only stdout.
# Env:
#   SONDE_BENCH_WARMUP  warm-up runs before timing starts (default 5)
#   SONDE_BENCH_RUNS    timed runs per case (default 20)
#
# Methodology is documented alongside the results in docs/benchmarks.md;
# this script is the "Method" column's source.
set -euo pipefail

WARMUP="${SONDE_BENCH_WARMUP:-5}"
RUNS="${SONDE_BENCH_RUNS:-20}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
SERVER_PID=""

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

have() { command -v "$1" >/dev/null 2>&1; }

# --- machine spec ------------------------------------------------------
os_name="$(uname -s)"
case "$os_name" in
Darwin)
  cpu="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)"
  time_v="/usr/bin/time -l"
  ;;
Linux)
  cpu="$(awk -F': ' '/model name/{print $2; exit}' /proc/cpuinfo 2>/dev/null || echo unknown)"
  time_v="/usr/bin/time -v"
  ;;
*)
  cpu="unknown"
  time_v=""
  ;;
esac
go_version="$(go version 2>/dev/null | awk '{print $3}')"
machine_spec="$os_name $(uname -m), ${cpu}, ${go_version}"

# --- build sonde ---------------------------------------------------------
SONDE_BIN="$WORK/sonde"
(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$SONDE_BIN" ./cmd/sonde)
sonde_version="$("$SONDE_BIN" version 2>&1 | head -1)"

# --- start the local bench server ---------------------------------------
SERVER_BIN="$WORK/bench-server"
(cd "$ROOT" && go build -o "$SERVER_BIN" ./scripts/bench)
"$SERVER_BIN" >"$WORK/server.out" 2>&1 &
SERVER_PID=$!
PORT=""
for _ in $(seq 1 50); do
  PORT="$(sed -n 's/^PORT=//p' "$WORK/server.out")"
  [ -n "$PORT" ] && break
  sleep 0.1
done
if [ -z "$PORT" ]; then
  echo "bench server did not report a port" >&2
  exit 1
fi
BASE_URL="http://127.0.0.1:$PORT/"
echo "bench server listening on $BASE_URL (pid $SERVER_PID)" >&2

# --- timing helpers --------------------------------------------------------
# Runs "$@" $WARMUP times unmeasured, then $RUNS times measured; prints
# "median_ms mean_ms" on stdout. Prefers hyperfine (consistent methodology,
# statistical outlier handling); falls back to a plain wall-clock loop.
time_case() {
  if have hyperfine; then
    local json="$WORK/hf-$$-$RANDOM.json"
    hyperfine --warmup "$WARMUP" -M "$RUNS" --export-json "$json" -- "$*" >/dev/null
    python3 - "$json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["results"][0]
print(f"{d['median']*1000:.2f} {d['mean']*1000:.2f}")
PY
  else
    # One python3 process runs the whole warm-up + timed loop, timing each
    # subprocess with perf_counter() in-process; this avoids adding a
    # second process spawn (for the timestamp itself) to every timed
    # iteration, which would swamp a sub-20ms command like `sonde version`.
    python3 - "$WARMUP" "$RUNS" "$*" <<'PY'
import statistics, subprocess, sys, time
warmup, runs, cmd = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3]
for _ in range(warmup):
    subprocess.run(cmd, shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
xs = []
for _ in range(runs):
    start = time.perf_counter()
    subprocess.run(cmd, shell=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    xs.append((time.perf_counter() - start) * 1000)
print(f"{statistics.median(xs):.2f} {statistics.mean(xs):.2f}")
PY
  fi
}

# Peak RSS (KB) of one run of "$@", via /usr/bin/time; empty if unsupported.
rss_case() {
  [ -z "$time_v" ] && return 0
  local log="$WORK/time-$$-$RANDOM.log"
  $time_v -- bash -c "$*" >/dev/null 2>"$log" || true
  case "$os_name" in
  Darwin) awk '/maximum resident set size/{print int($1/1024)}' "$log" ;;
  Linux) awk -F': ' '/Maximum resident set size/{print $2}' "$log" ;;
  esac
}

declare -a ROWS=()
add_row() {
  # tool, startup(ms), request(ms), rss(KB), version
  ROWS+=("$1|$2|$3|$4|$5")
}

bench_tool() {
  local label="$1" startup_cmd="$2" request_cmd="$3" version_cmd="$4"
  local startup req rss version
  startup="$(time_case "$startup_cmd")"
  req="$(time_case "$request_cmd")"
  rss="$(rss_case "$request_cmd")"
  version="$(eval "$version_cmd" 2>&1 | head -1 | tr -d '\r')"
  add_row "$label" "${startup% *}/${startup#* }" "${req% *}/${req#* }" "${rss:-n/a}" "$version"
}

# --- sonde ---------------------------------------------------------------
SONDE_REQ="$WORK/req.hurl"
printf 'GET %s\nHTTP 200\n' "$BASE_URL" >"$SONDE_REQ"
bench_tool "sonde" \
  "$SONDE_BIN version" \
  "$SONDE_BIN run $SONDE_REQ" \
  "echo '$sonde_version'"

# --- hurl ------------------------------------------------------------------
if have hurl; then
  bench_tool "hurl" "hurl --version" "hurl --test $SONDE_REQ" "hurl --version"
else
  echo "skip: hurl not installed" >&2
fi

# --- xh (not a file runner: one GET request stands in for "single request") -
if have xh; then
  bench_tool "xh" "xh --version" "xh --ignore-stdin GET $BASE_URL" "xh --version"
else
  echo "skip: xh not installed" >&2
fi

# --- newman (Postman CLI) --------------------------------------------------
if have newman; then
  COLLECTION="$WORK/collection.json"
  cat >"$COLLECTION" <<JSON
{
  "info": { "name": "bench", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json" },
  "item": [{ "name": "req", "request": { "method": "GET", "url": "$BASE_URL" } }]
}
JSON
  bench_tool "newman" "newman --version" "newman run $COLLECTION" "newman --version"
else
  echo "skip: newman not installed" >&2
fi

# --- bru (Bruno CLI) ---------------------------------------------------------
if have bru; then
  BRU_DIR="$WORK/bruno"
  mkdir -p "$BRU_DIR"
  cat >"$BRU_DIR/bruno.json" <<JSON
{ "version": "1", "name": "bench", "type": "collection" }
JSON
  cat >"$BRU_DIR/req.bru" <<BRU
meta {
  name: req
  type: http
  seq: 1
}

get {
  url: $BASE_URL
}
BRU
  bench_tool "bru" "bru --version" "bru run req.bru --cwd $BRU_DIR" "bru --version"
else
  echo "skip: bru not installed" >&2
fi

# --- report ----------------------------------------------------------------
report() {
  echo "Machine: $machine_spec"
  echo "Date: $(date -u +%Y-%m-%d)"
  echo
  echo "| Tool | Version | Startup median/mean (ms) | Single request median/mean (ms) | Peak RSS (KB) |"
  echo "|---|---|---|---|---|"
  for row in "${ROWS[@]}"; do
    IFS='|' read -r tool startup req rss version <<<"$row"
    echo "| $tool | $version | $startup | $req | $rss |"
  done
}
if [ -n "${1:-}" ]; then
  report | tee "$1"
else
  report
fi
