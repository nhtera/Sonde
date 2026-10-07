#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Extracts every CLI flag, environment variable and config-file key that an
# upstream source checkout (8.0.1 by default) defines, plus the request [Options] section
# keys (again, this time with usage counts), and writes
# internal/docs/testdata/cli-inventory.tsv as
# "kind<TAB>name<TAB>short<TAB>arg<TAB>usage":
#   - flags:   long flag (e.g. "--connect-timeout"), short ("-m" or ""),
#              arg placeholder (e.g. "SECONDS", or "" for a boolean flag),
#              usage = occurrences of the long or short form across vendored
#              testdata/conformance/hurl/**/*.sh scripts.
#   - env:     env var name (e.g. "HURL_JOBS", or the literal pattern
#              "HURL_SECRET_name" / "HURL_VARIABLE_name"), no short/arg,
#              usage = occurrences across the same .sh scripts.
#   - options: request [Options] section key (e.g. "connect-timeout"), no
#              short/arg, usage = number of vendored .hurl files that use the
#              key inside an [Options] section.
#
# Flags and the request-option key list come from Hurl's own option
# definitions (packages/hurl/src/cli/options/commands.rs,
# packages/hurl_core/src/parser/option.rs); env vars come from
# packages/hurl/src/cli/options/env_vars.rs (context.rs up to 8.0.1).
#
# Usage: scripts/gen-cli-inventory.sh [--ref next] [HURL_SRC_DIR]
# The default ref is tag 8.0.1, counted against the vendored corpus and
# written to cli-inventory.tsv. `--ref next` reads the commit pinned in
# internal/conformance/manifest-next.yaml, counts usage in the snapshot
# `make conformance-next` syncs to the user cache (or $SONDE_NEXT_CORPUS)
# and writes cli-inventory-next.tsv. With no HURL_SRC_DIR, the ref is
# sparse-fetched into a temp dir cleaned on exit.
set -euo pipefail
export LC_ALL=C

root="$(git rev-parse --show-toplevel)"
. "$root/scripts/inventory-ref.sh"
parse_inventory_ref "$@"
out="$root/internal/docs/testdata/cli-inventory${inventory_suffix}.tsv"
corpus_dir="$inventory_corpus"

src="$inventory_src"
work=""
if [ -z "$src" ]; then
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  fetch_inventory_ref "$work/hurl" \
    '/packages/hurl/src/cli/options/' '/packages/hurl_core/src/parser/option.rs'
  src="$work/hurl"
fi

commands_rs="$src/packages/hurl/src/cli/options/commands.rs"
# The file that declares the HURL_* names: context.rs up to 8.0.1, then
# env_vars.rs.
context_rs="$src/packages/hurl/src/cli/options/context.rs"
if ! grep -q 'const HURL_' "$context_rs" 2>/dev/null; then
  context_rs="$src/packages/hurl/src/cli/options/env_vars.rs"
fi
config_rs="$src/packages/hurl/src/cli/options/config_file/mod.rs"
option_rs="$src/packages/hurl_core/src/parser/option.rs"
for f in "$commands_rs" "$context_rs" "$config_rs" "$option_rs"; do
  if [ ! -f "$f" ]; then
    echo "gen-cli-inventory: missing $f" >&2
    exit 1
  fi
done

work2="$(mktemp -d)"
trap 'rm -rf "$work2" ${work:+"$work"}' EXIT

# --- flags: one function per clap::Arg in commands.rs. Join each function's
# body onto a single line, then pull long/short/value_name/boolean out of it.
flags_raw="$work2/flags-raw.tsv"
: > "$flags_raw"
while IFS= read -r block; do
  long="$(printf '%s' "$block" | grep -oE '\.long\("[^"]+"\)' | head -1 | sed -E 's/\.long\("([^"]+)"\)/\1/' || true)"
  [ -n "$long" ] || continue # skip the "input_files" positional
  short="$(printf '%s' "$block" | grep -oE "\\.short\\('.'\\)" | head -1 | sed -E "s/\\.short\\('(.)'\\)/\\1/" || true)"
  arg="$(printf '%s' "$block" | grep -oE '\.value_name\("[^"]+"\)' | head -1 | sed -E 's/\.value_name\("([^"]+)"\)/\1/' || true)"
  if printf '%s' "$block" | grep -q 'ArgAction::SetTrue'; then
    arg=""
  fi
  printf 'flags\t--%s\t%s\t%s\n' "$long" "${short:+-$short}" "$arg" >> "$flags_raw"
done < <(awk '
  /^pub fn [a-zA-Z0-9_]+\(\) -> clap::Arg \{/ { if (block!="") print block; block=$0; next }
  block != "" { block = block " " $0 }
  END { if (block!="") print block }
' "$commands_rs")

# --- env: named HURL_* consts (const HURL_X: &str = "HURL_Y";), the two
# HURL_SECRET_/HURL_VARIABLE_ prefixes (as the literal patterns
# "HURL_SECRET_name" / "HURL_VARIABLE_name"), and the non-HURL_-prefixed vars
# Hurl reads directly (NO_COLOR, CI, TF_BUILD, XDG_CONFIG_HOME) — asserted
# present in context.rs so source drift fails loudly instead of silently
# dropping an entry. (8.0.1 declares them in context.rs, later versions in
# env_vars.rs.)
env_raw="$work2/env-raw.tsv"
: > "$env_raw"
grep -oE '^(pub )?const HURL_[A-Z0-9_]+: &str = "[A-Z0-9_]+";' "$context_rs" \
  | sed -E 's/.*= "([A-Z0-9_]+)";/\1/' \
  | grep -v -E '_$' \
  >> "$env_raw.names" || true
{
  echo "HURL_SECRET_name"
  echo "HURL_VARIABLE_name"
} >> "$env_raw.names"
for extra in NO_COLOR CI TF_BUILD XDG_CONFIG_HOME; do
  if ! grep -q "\"$extra\"" "$context_rs"; then
    echo "gen-cli-inventory: expected ${context_rs##*/} to reference \"$extra\"" >&2
    exit 1
  fi
  echo "$extra" >> "$env_raw.names"
done
sort -u "$env_raw.names" | sed 's/^/env\t/' | awk -F'\t' '{print $0"\t\t"}' > "$env_raw"

# --- config: top-level match arms in config_file/mod.rs, e.g. `"verbose" => {`
# (8.0.1) or `"verbose" => parse_option_verbose(reader, options),`.
config_raw="$work2/config-raw.tsv"
grep -oE '^        "[a-z0-9.-]+" => (\{$|parse_option_)' "$config_rs" \
  | sed -E 's/^ *"([a-z0-9.-]+)".*/config\t\1\t\t/' | sort -u > "$config_raw"

# --- options: request [Options] section keys, same extraction as
# gen-grammar-inventory.sh's "options" kind (kept independent so this script
# is runnable standalone).
options_raw="$work2/options-raw.tsv"
grep -oE '"[a-z0-9.-]+" => option_[a-z0-9_]+\(reader\)\?,' "$option_rs" \
  | sed -E 's/"([a-z0-9.-]+)".*/options\t\1\t\t/' | sort -u > "$options_raw"

if [ ! -d "$corpus_dir" ]; then
  echo "gen-cli-inventory: missing conformance corpus $corpus_dir" >&2
  exit 1
fi

sh_files=()
while IFS= read -r -d '' f; do sh_files+=("$f"); done \
  < <(find "$corpus_dir" -name '*.sh' -print0 | sort -z)
hurl_files=()
while IFS= read -r -d '' f; do hurl_files+=("$f"); done \
  < <(find "$corpus_dir" -name '*.hurl' -print0 | sort -z)

# --- flag usage: count long/short-form token occurrences (bare or "=value")
# across the vendored .sh scripts.
lookup="$work2/flag-lookup.tsv"
awk -F'\t' '{ print $2"\t"$2; if ($3!="") print $3"\t"$2 }' "$flags_raw" > "$lookup"
flag_counts="$work2/flag-counts.tsv"
if [ "${#sh_files[@]}" -gt 0 ]; then
  awk '
    NR==FNR { map[$1]=$2; next }
    {
      for (i=1;i<=NF;i++) {
        w=$i
        if (w in map) { cnt[map[w]]++; continue }
        eq = index(w,"=")
        if (eq>1) {
          pre = substr(w,1,eq-1)
          if (pre in map) cnt[map[pre]]++
        }
      }
    }
    END { for (k in cnt) print k"\t"cnt[k] }
  ' "$lookup" "${sh_files[@]}" | sort > "$flag_counts"
else
  : > "$flag_counts"
fi

# --- env usage: same token scan, normalizing "$VAR", "${VAR}" and
# "VAR=value" forms; prefix vars count any token starting with the prefix.
env_counts="$work2/env-counts.tsv"
if [ "${#sh_files[@]}" -gt 0 ]; then
  awk '
    { for (i=1;i<=NF;i++) {
        w=$i
        gsub(/[${}]/, "", w)
        eq = index(w,"=")
        if (eq>1) w = substr(w,1,eq-1)
        if (w ~ /^HURL_SECRET_./) { cnt["HURL_SECRET_name"]++; continue }
        if (w ~ /^HURL_VARIABLE_./) { cnt["HURL_VARIABLE_name"]++; continue }
        cnt[w]++
      }
    }
    END { for (k in cnt) print k"\t"cnt[k] }
  ' "${sh_files[@]}" | sort > "$env_counts"
else
  : > "$env_counts"
fi

# --- option usage: distinct .hurl files with the key set inside an
# [Options] section.
option_counts="$work2/option-counts.tsv"
if [ "${#hurl_files[@]}" -gt 0 ]; then
  awk '
    FNR==1 { inopt=0 }
    /^\[Options\][ \t]*$/ { inopt=1; next }
    /^\[/ { inopt=0 }
    {
      if (inopt && match($0, /^[A-Za-z0-9._-]+:/)) {
        key = substr($0, 1, RLENGTH-1)
        pairkey = FILENAME SUBSEP key
        if (!(pairkey in seen)) { seen[pairkey]=1; filecount[key]++ }
      } else if (inopt) {
        inopt=0
      }
    }
    END { for (k in filecount) print k"\t"filecount[k] }
  ' "${hurl_files[@]}" | sort > "$option_counts"
else
  : > "$option_counts"
fi

# --- join usage back onto each raw table (default 0), then concatenate.
join_usage() {
  local raw="$1" counts="$2"
  awk -F'\t' '
    NR==FNR { u[$1]=$2; next }
    { print $1"\t"$2"\t"$3"\t"$4"\t"(($2 in u) ? u[$2] : 0) }
  ' "$counts" "$raw"
}

{
  join_usage "$flags_raw" "$flag_counts"
  join_usage "$env_raw" "$env_counts"
  join_usage "$options_raw" "$option_counts"
} | sort -u > "$out"

count="$(wc -l < "$out" | tr -d ' ')"
echo "gen-cli-inventory: wrote $count entries to ${out#"$root"/}" >&2
echo "gen-cli-inventory: config keys (not usage-tracked): $(cut -f2 "$config_raw" | paste -sd, -)" >&2
