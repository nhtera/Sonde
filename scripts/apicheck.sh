#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Fails when the public Go API of the module (every package outside
# internal/) changed incompatibly since a baseline git ref.
# Usage: scripts/apicheck.sh [BASE_REF]   (default: contents of .api-baseline)
# Until the first v1 tag exists the check is skipped; after that a
# baseline ref that can't be found fails. Needs APIDIFF (default:
# bin/apidiff).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
base=${1:-$(tr -d '[:space:]' < .api-baseline)}
apidiff=${APIDIFF:-$root/bin/apidiff}
module=$(go list -m)

if ! git rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
  # Before the first v1 tag there is nothing to compare with; once one
  # exists, a baseline that can't be found (a typo) is an error.
  if [ -z "$(git tag --list 'v1.*')" ]; then
    echo "apicheck: no v1 release tagged yet ($base); skipping"
    exit 0
  fi
  echo "apicheck: baseline $base not found" >&2
  exit 1
fi

tmp=$(mktemp -d)
cleanup() {
  git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT
git worktree add --quiet --detach "$tmp/base" "$base"

# apidiff names every internal package it skips on stderr; keep the rest.
quiet() { "$@" 2> >(grep -v '^Ignoring internal package' >&2); }

# Export data depends on the compiler: write both sides with this toolchain.
(cd "$tmp/base" && quiet "$apidiff" -m -w "$tmp/base.export" "$module")
out=$(quiet "$apidiff" -m -incompatible "$tmp/base.export" "$module")
if [ -n "$out" ]; then
  printf 'apicheck: incompatible API changes since %s:\n%s\n' "$base" "$out"
  exit 1
fi
echo "apicheck: no incompatible API changes since $base"
