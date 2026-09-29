#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Installs (install) or checks (verify) the pinned desktop build tools
# listed in tools.sha256 into BIN (default: <repo>/bin).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
bin="${BIN:-$(cd "$here/../.." && pwd)/bin}"
mode="${1:-verify}"

check() { # binary module version sum
  local got
  got="$(go version -m "$1" 2>/dev/null | awk -v m="$2" '$1 == "mod" && $2 == m { print $3, $4 }')"
  [ "$got" = "$3 $4" ]
}

status=0
while read -r pkg version sum; do
  case "$pkg" in ''|'#'*) continue ;; esac
  name="${pkg##*/}"
  module="${pkg%/cmd/*}"
  if [ "$mode" = install ] && ! check "$bin/$name" "$module" "$version" "$sum"; then
    # Built with the repo's toolchain (go.mod), so the binding generator
    # reads the same standard library the app compiles against.
    (cd "$here/../.." && GOBIN="$bin" GOTOOLCHAIN="$(go env GOVERSION)" GOFLAGS=-mod=mod go install "$pkg@$version")
  fi
  if check "$bin/$name" "$module" "$version" "$sum"; then
    echo "ok $name $version"
  else
    echo "$bin/$name is not $module $version with $sum" >&2
    status=1
  fi
done < "$here/tools.sha256"
exit "$status"
