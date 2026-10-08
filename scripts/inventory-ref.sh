# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Sourced by gen-cli-inventory.sh and gen-grammar-inventory.sh: picks the
# upstream ref an inventory is generated from.
#
# parse_inventory_ref "$@" sets
#   inventory_ref     the tag or commit (8.0.1, or the next snapshot's sha)
#   inventory_suffix  "" for 8.0.1, "-next" for the snapshot
#   inventory_src     the HURL_SRC_DIR argument, if any
#   inventory_corpus  the test scripts usage is counted in
# fetch_inventory_ref DIR PATTERN... sparse-fetches inventory_ref into DIR.

parse_inventory_ref() {
  inventory_ref="8.0.1"
  inventory_suffix=""
  inventory_src=""
  inventory_corpus="$root/testdata/conformance/hurl"
  while [ $# -gt 0 ]; do
    case "$1" in
      --ref)
        [ "${2:-}" = "next" ] || { echo "inventory: --ref takes 'next'" >&2; exit 2; }
        inventory_ref="$(sed -n 's/^# upstream-commit: \([0-9a-f]*\)$/\1/p' "$root/internal/conformance/manifest-next.yaml")"
        [ -n "$inventory_ref" ] || { echo "inventory: no upstream-commit in manifest-next.yaml" >&2; exit 1; }
        inventory_suffix="-next"
        inventory_corpus="${SONDE_NEXT_CORPUS:-$(next_cache_dir)/hurl}"
        shift 2
        ;;
      *)
        inventory_src="$1"
        shift
        ;;
    esac
  done
}

# next_cache_dir is where `make conformance-next` syncs the snapshot
# (os.UserCacheDir()/sonde-conformance/next).
next_cache_dir() {
  local base
  case "$(uname -s)" in
    Darwin) base="$HOME/Library/Caches" ;;
    *) base="${XDG_CACHE_HOME:-$HOME/.cache}" ;;
  esac
  printf '%s/sonde-conformance/next' "$base"
}

fetch_inventory_ref() {
  local dir="$1"
  shift
  git init --quiet "$dir"
  git -C "$dir" remote add origin https://github.com/Orange-OpenSource/hurl.git
  git -C "$dir" sparse-checkout set --no-cone "$@" >&2
  git -C "$dir" fetch --quiet --depth 1 --filter=blob:none origin "$inventory_ref" >&2
  git -C "$dir" -c advice.detachedHead=false checkout --quiet FETCH_HEAD >&2
}
