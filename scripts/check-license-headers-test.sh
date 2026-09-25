#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Self-test for check-license-headers.sh: a file with the header passes,
# a file without it fails.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/check-license-headers.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf '// Copyright 2026 The Sonde Authors\n// SPDX-License-Identifier: Apache-2.0\n\npackage x\n' > "$tmp/good.go"
printf 'package x\n' > "$tmp/bad.go"

"$script" "$tmp/good.go" > /dev/null || { echo "FAIL: header present but check failed" >&2; exit 1; }
if "$script" "$tmp/bad.go" > /dev/null 2>&1; then
  echo "FAIL: header missing but check passed" >&2
  exit 1
fi
echo "check-license-headers self-test ok"
