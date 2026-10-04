#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Checks the macOS update archive: exactly one top-level entry, Sonde.app,
# and no AppleDouble (._*) or __MACOSX entry anywhere. The updater unpacks
# it and expects the app at its root.
# Usage: check-app-zip.sh ZIP
set -euo pipefail

zip="$1"
entries="$(unzip -Z1 "$zip")"
top="$(printf '%s\n' "$entries" | cut -d/ -f1 | sort -u)"
if [ "$top" != "Sonde.app" ]; then
  echo "$zip: top-level entries are not exactly Sonde.app:" >&2
  printf '%s\n' "$top" >&2
  exit 1
fi
if printf '%s\n' "$entries" | grep -Eq '(^|/)(\._|__MACOSX(/|$))'; then
  echo "$zip: AppleDouble or __MACOSX entries:" >&2
  printf '%s\n' "$entries" | grep -E '(^|/)(\._|__MACOSX(/|$))' >&2
  exit 1
fi
echo "ok $(basename "$zip"): one Sonde.app"
