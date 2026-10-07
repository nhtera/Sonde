#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Vendors the reference implementation's integration test trees (the
# conformance oracle). Usage: scripts/sync-hurl-conformance.sh [REF] [DEST]
#
#   REF   a release tag (default 8.0.1) or a full 40-hex commit sha
#   DEST  the conformance root to write (default testdata/conformance); it
#         receives hurl/, hurlfmt/ and integration/ (the upstream runner)
#
# Upstream files are copied unchanged; bump the tag deliberately after
# reading the upstream changelog. A sha ref is checked against the fetched
# HEAD, so a moved or mistyped pin fails instead of syncing something else.
set -euo pipefail

ref="${1:-8.0.1}"
repo="https://github.com/Orange-OpenSource/hurl.git"
root="$(git rev-parse --show-toplevel)"
dest="${2:-$root/testdata/conformance}"

is_sha=false
if [[ "$ref" =~ ^[0-9a-f]{40}$ ]]; then
  is_sha=true
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git init --quiet "$work/hurl"
git -C "$work/hurl" remote add origin "$repo"
git -C "$work/hurl" sparse-checkout set --no-cone \
  '/integration/hurl/' '/integration/hurlfmt/' \
  '/integration/test_script.py' '/integration/term.py' '/integration/test_pattern.py' \
  '/bin/requirements-frozen.txt' '/LICENSE'
git -C "$work/hurl" fetch --quiet --depth 1 --filter=blob:none origin "$ref"
git -C "$work/hurl" -c advice.detachedHead=false checkout --quiet FETCH_HEAD
sha="$(git -C "$work/hurl" rev-parse HEAD)"
if $is_sha && [ "$sha" != "$ref" ]; then
  echo "sync: fetched $sha, want $ref" >&2
  exit 1
fi

# write_source DIR: records provenance and per-directory counts in DIR/SOURCE.
write_source() {
  local dir="$1"
  {
    echo "repository: $repo"
    if $is_sha; then
      echo "ref: $ref"
    else
      echo "tag: $ref"
    fi
    echo "commit: $sha"
    echo "license: Apache-2.0 (see LICENSE)"
    echo "files unchanged from upstream; synced by scripts/sync-hurl-conformance.sh"
    echo
    echo "counts (.hurl / .sh):"
    for sub in "$dir"/tests_*/; do
      name="$(basename "$sub")"
      hurl_count="$(find "$sub" -name '*.hurl' | wc -l | tr -d ' ')"
      sh_count="$(find "$sub" -name '*.sh' | wc -l | tr -d ' ')"
      echo "  $name: $hurl_count / $sh_count"
    done
  } > "$dir/SOURCE"
}

mkdir -p "$dest"
for tree in hurl hurlfmt; do
  rm -rf "${dest:?}/$tree"
  mkdir -p "$dest/$tree"
  cp -R "$work/hurl/integration/$tree/." "$dest/$tree/"
  cp "$work/hurl/LICENSE" "$dest/$tree/LICENSE"
done
mkdir -p "$dest/hurl/bin"
cp "$work/hurl/bin/requirements-frozen.txt" "$dest/hurl/bin/"
write_source "$dest/hurl"
write_source "$dest/hurlfmt"

rm -rf "${dest:?}/integration"
mkdir -p "$dest/integration"
for f in test_script.py term.py test_pattern.py; do
  cp "$work/hurl/integration/$f" "$dest/integration/$f"
done
cp "$work/hurl/LICENSE" "$dest/integration/LICENSE"
{
  echo "repository: $repo"
  echo "commit: $sha"
  echo "license: Apache-2.0 (see LICENSE)"
  echo "upstream script runner (test_script.py, term.py, test_pattern.py), unchanged"
} > "$dest/integration/SOURCE"

echo "synced upstream $ref ($sha) into ${dest#"$root"/}"
