#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# CI stand-in for a Linux machine (ubuntu-24.04, libfuse2): checks, with a
# real AppImage mount, what the app's in-place install relies on:
#   - started by the AppImage runtime, the app finds that it runs from that
#     very file (sonde-desktop --update-install-check);
#   - run from the same mount with another APPIMAGE (inherited or forged),
#     it refuses;
#   - the file can be replaced (renamed over) while it is mounted, the old
#     mount still reads, and the new file keeps mode 0755.
# Usage: ci-appimage-update.sh APPIMAGE
set -euo pipefail

src="$(readlink -f "$1")"
t="$(mktemp -d)"
mounter=
trap 'kill ${mounter:-} 2>/dev/null || true; rm -rf "$t"' EXIT
fail() { echo "FAIL: $1" >&2; exit 1; }

# The image in a folder only its owner can write, as the check requires.
mkdir -p "$t/apps"
chmod 755 "$t/apps"
img="$t/apps/Sonde.AppImage"
cp "$src" "$img"
chmod 755 "$img"

# How this runtime mounts its image, for the log.
"$img" --appimage-mount > "$t/mountpoint" &
mounter=$!
for _ in $(seq 50); do [ -s "$t/mountpoint" ] && break; sleep 0.1; done
mnt="$(head -1 "$t/mountpoint")"
[ -d "$mnt/usr/bin" ] || fail "no mount at '$mnt'"
echo "mountinfo: $(grep -F " $mnt " /proc/self/mountinfo || echo none)"

# 1. Started by the runtime: the check passes.
"$img" --update-install-check || fail "the AppImage runtime's own launch was refused"

# 2. The same mount, a foreign APPIMAGE: refused.
bin="$(find "$mnt/usr/bin" -name 'sonde-desktop*' -type f | head -1)"
cp "$src" "$t/Other.AppImage"
if APPIMAGE="$t/Other.AppImage" APPDIR="$mnt" "$bin" --update-install-check; then
  fail "a foreign APPIMAGE was accepted"
fi
# The same check with this image's own path, from outside the runtime's
# process: mountinfo names the image (the step that settles the mountinfo
# claim on Ubuntu).
APPIMAGE="$img" APPDIR="$mnt" "$bin" --update-install-check || fail "mountinfo does not name the image"

# 3. Replaced while mounted, as the installer does (a new name, then a
# rename over the old one).
cp "$src" "$t/apps/.Sonde-update-ci.AppImage"
chmod 755 "$t/apps/.Sonde-update-ci.AppImage"
mv -f "$t/apps/.Sonde-update-ci.AppImage" "$img"
[ "$(stat -c %a "$img")" = 755 ] || fail "the new image's mode is $(stat -c %a "$img")"
cat "$bin" > /dev/null || fail "the old mount no longer reads after the rename"
"$img" --update-install-check || fail "the new image's check"
echo "appimage update: ok"
