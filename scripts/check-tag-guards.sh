#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# The CLI's releases find their previous tag with `git describe` and
# GoReleaser; other release trains (editors/vscode/v*, desktop/v*) share the
# tag namespace and must never be picked. This checks that:
#   1. every non-CLI tag prefix (from the release workflows' tag triggers,
#      plus desktop/ reserved for the desktop app) is in .goreleaser.yaml
#      git.ignore_tags;
#   2. every `git describe` in the workflows excludes each prefix, or
#      matches only CLI tags (--match 'v[0-9]*');
#   3. in a throwaway clone, a desktop/v0.0.0-test tag (and one per prefix)
#      on HEAD leaves the snapshot job's describe output unchanged.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"
status=0
fail() { echo "tag guard: $*" >&2; status=1; }

# The tag globs of the workflows' push triggers, inline (tags: ["a", "b"])
# or as a block list (tags: then "- a" lines).
tag_globs() {
  awk '
    /^[[:space:]]*tags:[[:space:]]*\[/ { sub(/.*\[/, ""); sub(/\].*/, ""); n = split($0, a, ","); for (i = 1; i <= n; i++) print a[i]; next }
    /^[[:space:]]*tags:[[:space:]]*$/ { list = 1; next }
    list && /^[[:space:]]*-/ { sub(/^[[:space:]]*-[[:space:]]*/, ""); print; next }
    { list = 0 }
  ' "$@" | tr -d "\"' " | grep -v '^$'
}

# Non-CLI prefixes: "editors/vscode/v*" -> "editors/vscode/".
prefixes=(desktop/)
while IFS= read -r glob; do
  case "$glob" in */*) prefixes+=("${glob%/*}/") ;; esac
done < <(tag_globs .github/workflows/*.yml)
sorted=()
while IFS= read -r p; do sorted+=("$p"); done < <(printf '%s\n' "${prefixes[@]}" | sort -u)
prefixes=("${sorted[@]}")

# The entries of .goreleaser.yaml git.ignore_tags.
ignored="$(awk '
  /^git:/ { git = 1; next }
  /^[^[:space:]#]/ { git = 0 }
  git && /^[[:space:]]+ignore_tags:/ { list = 1; next }
  list && /^[[:space:]]+-/ { sub(/^[[:space:]]+-[[:space:]]*/, ""); gsub(/"/, ""); print; next }
  { list = 0 }
' .goreleaser.yaml)"
for p in "${prefixes[@]}"; do
  printf '%s\n' "$ignored" | grep -qxF "${p}*" || fail ".goreleaser.yaml git.ignore_tags lacks \"${p}*\""
done

while IFS= read -r line; do
  file="${line%%:*}"
  cmd="${line#*:}"
  case "$cmd" in *"--match 'v[0-9]*'"*) continue ;; esac
  for p in "${prefixes[@]}"; do
    case "$cmd" in *"--exclude '${p}*'"*) ;; *) fail "$file: git describe without --exclude '${p}*': ${cmd#"${cmd%%[![:space:]]*}"}" ;; esac
  done
done < <(grep -Ho 'git describe[^)]*' .github/workflows/*.yml)

# The snapshot job's lookup, run before and after adding the tags.
describe="$(grep -ho "git describe --tags --abbrev=0[^)]*" .github/workflows/ci.yml | head -n 1)"
if [ -z "$describe" ]; then
  fail "no git describe in ci.yml"
elif git describe --tags --abbrev=0 >/dev/null 2>&1; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  git clone -q --no-checkout "$root" "$tmp/repo"
  before="$(cd "$tmp/repo" && eval "$describe")"
  for p in "${prefixes[@]}"; do
    git -C "$tmp/repo" tag "${p}v0.0.0-test" HEAD
  done
  after="$(cd "$tmp/repo" && eval "$describe")"
  [ "$before" = "$after" ] || fail "a ${prefixes[*]} tag on HEAD changed the CLI's previous tag: $before -> $after"
else
  echo "tag guard: no tags in this clone; skipping the describe check (fetch-depth: 0 runs it)" >&2
fi

[ "$status" -eq 0 ] && echo "tag guards ok (${prefixes[*]})"
exit "$status"
