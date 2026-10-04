#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# check-app-zip.sh passes an archive of Sonde.app alone, and refuses a
# __MACOSX folder, an AppleDouble file and a second top-level entry.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
t="$(mktemp -d)"
trap 'rm -rf "$t"' EXIT
cd "$t"
mkdir -p Sonde.app/Contents/MacOS __MACOSX
echo app > Sonde.app/Contents/MacOS/sonde-desktop
echo meta > __MACOSX/._Sonde.app
echo meta > Sonde.app/Contents/._Info.plist
echo other > README

fail() { echo "FAIL: $1" >&2; exit 1; }
zip -qr ok.zip Sonde.app -x 'Sonde.app/Contents/._Info.plist'
"$here/check-app-zip.sh" ok.zip >/dev/null || fail "Sonde.app alone refused"
zip -qr macosx.zip Sonde.app __MACOSX -x 'Sonde.app/Contents/._Info.plist'
! "$here/check-app-zip.sh" macosx.zip >/dev/null 2>&1 || fail "__MACOSX passed"
zip -qr double.zip Sonde.app
! "$here/check-app-zip.sh" double.zip >/dev/null 2>&1 || fail "an AppleDouble file passed"
zip -qr two.zip Sonde.app README -x 'Sonde.app/Contents/._Info.plist'
! "$here/check-app-zip.sh" two.zip >/dev/null 2>&1 || fail "a second top-level entry passed"
echo "check-app-zip: ok"
