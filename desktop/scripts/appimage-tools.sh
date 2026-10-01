#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Puts a pinned, checksum-verified linuxdeploy in the AppImage build
# folder, where `wails3 generate appimage` uses it instead of downloading
# the moving "continuous" build. (The AppRun Wails fetches from the
# archived AppImageKit is not pinned: see docs/security.md.)
# Usage: appimage-tools.sh BUILD_DIR
set -euo pipefail

dir="$1"
version="1-alpha-20251107-1"
case "$(uname -m)" in
  x86_64) arch=x86_64; sum=c20cd71e3a4e3b80c3483cef793cda3f4e990aca14014d23c544ca3ce1270b4d ;;
  *) echo "the AppImage is built for x86_64 only" >&2; exit 1 ;;
esac
mkdir -p "$dir"
out="$dir/linuxdeploy-$arch.AppImage"
curl -fsSL -o "$out.part" "https://github.com/linuxdeploy/linuxdeploy/releases/download/$version/linuxdeploy-$arch.AppImage"
echo "$sum  $out.part" | sha256sum -c -
mv "$out.part" "$out"
chmod +x "$out"
