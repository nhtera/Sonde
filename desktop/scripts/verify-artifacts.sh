#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Checks release artifacts before they are signed: each one is listed in
# the build job's digests file with its SHA-256, and (with REPO set) has a
# build provenance attestation made by this repository's release workflow.
# Fails closed: a file not listed, a digest that differs, an attestation
# missing or from elsewhere.
# Usage: verify-artifacts.sh DIGESTS FILE...
set -euo pipefail

digests="$1"
shift
[ "$#" -gt 0 ] || { echo "no artifact to verify" >&2; exit 1; }
sha() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{print $1}'; }

status=0
for f in "$@"; do
  name="$(basename "$f")"
  want="$(awk -v n="$name" '$2 == n || $2 == "*"n { print $1 }' "$digests")"
  if [ -z "$want" ]; then
    echo "$name: not in $digests" >&2
    status=1
    continue
  fi
  got="$(sha "$f")"
  if [ "$got" != "$want" ]; then
    echo "$name: SHA-256 $got, the build recorded $want" >&2
    status=1
    continue
  fi
  if [ -n "${REPO:-}" ]; then
    if ! gh attestation verify "$f" --repo "$REPO" --signer-workflow "$REPO/.github/workflows/release-desktop.yml" >/dev/null; then
      echo "$name: no build provenance attestation from $REPO's release workflow" >&2
      status=1
      continue
    fi
  fi
  echo "ok $name"
done
exit "$status"
