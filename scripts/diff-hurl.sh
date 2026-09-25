#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Records reference assert results for the evaluation differential tests.
# Runs every testdata/eval/diff/*.hurl file with the given hurl binary
# against the fixture server of internal/evaldiff and rewrites the matching
# .json files; `go test ./internal/evaldiff` then compares sonde with them.
#
# Usage: scripts/diff-hurl.sh [HURL_BIN]   (default: hurl on PATH)
set -euo pipefail

hurl_bin=${1:-$(command -v hurl || true)}
if [[ -z "$hurl_bin" || ! -x "$hurl_bin" ]]; then
  echo "error: hurl binary not found; pass its path as the first argument" >&2
  exit 1
fi
hurl_bin=$(cd "$(dirname "$hurl_bin")" && pwd)/$(basename "$hurl_bin")
"$hurl_bin" --version | head -1

cd "$(git rev-parse --show-toplevel)"
go test ./internal/evaldiff -run TestDiff -count=1 -reference="$hurl_bin"
git --no-pager diff --stat -- testdata/eval/diff
