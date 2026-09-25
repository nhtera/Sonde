#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Vendors Hurl's integration test tree (the conformance oracle) into
# testdata/conformance/hurl. Usage: scripts/sync-hurl-conformance.sh [TAG]
# Upstream files are copied unchanged; bump the tag deliberately after
# reading Hurl's changelog.
set -euo pipefail

tag="${1:-8.0.1}"
repo="https://github.com/Orange-OpenSource/hurl.git"
root="$(git rev-parse --show-toplevel)"
dest="$root/testdata/conformance/hurl"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" --filter=blob:none --sparse "$repo" "$work/hurl"
git -C "$work/hurl" sparse-checkout set --no-cone \
  '/integration/hurl/' '/bin/requirements-frozen.txt' '/LICENSE'
sha="$(git -C "$work/hurl" rev-parse HEAD)"

rm -rf "$dest"
mkdir -p "$dest/bin"
cp -R "$work/hurl/integration/hurl/." "$dest/"
cp "$work/hurl/bin/requirements-frozen.txt" "$dest/bin/"
cp "$work/hurl/LICENSE" "$dest/LICENSE"

{
  echo "repository: $repo"
  echo "tag: $tag"
  echo "commit: $sha"
  echo "license: Apache-2.0 (see LICENSE)"
  echo "files unchanged from upstream; synced by scripts/sync-hurl-conformance.sh"
  echo
  echo "counts (.hurl / .sh):"
  for dir in "$dest"/tests_*/; do
    name="$(basename "$dir")"
    hurl_count="$(find "$dir" -name '*.hurl' | wc -l | tr -d ' ')"
    sh_count="$(find "$dir" -name '*.sh' | wc -l | tr -d ' ')"
    echo "  $name: $hurl_count / $sh_count"
  done
} > "$dest/SOURCE"

echo "synced Hurl $tag ($sha) into ${dest#"$root"/}"
