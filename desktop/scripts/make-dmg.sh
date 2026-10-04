#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Makes the release disk image from a (signed) Sonde.app: the app, an
# Applications link and the background that guides the drag, laid out
# by dmgbuild (build/darwin/dmg-requirements.txt) with no Finder needed.
# Usage: make-dmg.sh APP DMG
set -euo pipefail

app="$1"
out="$2"
here="$(cd "$(dirname "$0")/../build/darwin" && pwd)"
[ "$(basename "$app")" = Sonde.app ] || { echo "make-dmg.sh: expected a Sonde.app, got $app" >&2; exit 1; }
rm -f "$out"
dmgbuild -s "$here/dmg-settings.py" -D app="$app" -D build="$here" Sonde "$out"
