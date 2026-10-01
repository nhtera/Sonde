#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# verify-artifacts.sh signs off listed, unchanged files only: a changed
# file, a file not listed and no file at all are refused.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
t="$(mktemp -d)"
trap 'rm -rf "$t"' EXIT
cd "$t"
echo a > one.bin
echo b > two.bin
if command -v sha256sum >/dev/null; then sha256sum one.bin two.bin; else shasum -a 256 one.bin two.bin; fi > digests.txt

fail() { echo "FAIL: $1" >&2; exit 1; }
"$here/verify-artifacts.sh" digests.txt one.bin two.bin >/dev/null || fail "listed, unchanged files refused"
echo changed >> two.bin
! "$here/verify-artifacts.sh" digests.txt one.bin two.bin >/dev/null 2>&1 || fail "a changed file passed"
echo c > three.bin
! "$here/verify-artifacts.sh" digests.txt three.bin >/dev/null 2>&1 || fail "a file not listed passed"
! "$here/verify-artifacts.sh" digests.txt >/dev/null 2>&1 || fail "no file passed"
echo "verify-artifacts: ok"
