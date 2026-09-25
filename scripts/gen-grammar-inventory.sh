#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Extracts the names of every query, filter, predicate, template function and
# request [Options] section key from a Hurl 8.0.1 source checkout, and writes
# a sorted "kind<TAB>name" list to internal/docs/testdata/grammar-inventory.tsv.
#
# The parser source (packages/hurl_core/src/parser/*.rs) is the primary,
# authoritative source: it is what the real `hurl` binary accepts. docs/
# grammar.md is cross-checked and any name it documents but the parser does
# not implement (or vice versa) is reported on stderr — grammar.md is known
# to lag the parser in places (e.g. Hurl 8.0.1's grammar.md omits the
# `rawbytes` and `redirects` queries and the deprecated `decode`/`format`
# filter aliases, and documents a `charsetEncode` filter the parser does not
# implement).
#
# Usage: scripts/gen-grammar-inventory.sh [HURL_SRC_DIR]
# With no argument, sparse-clones tag 8.0.1 into a temp dir cleaned on exit.
set -euo pipefail
export LC_ALL=C

root="$(git rev-parse --show-toplevel)"
out="$root/internal/docs/testdata/grammar-inventory.tsv"

src="${1:-}"
work=""
if [ -z "$src" ]; then
  tag="8.0.1"
  repo="https://github.com/Orange-OpenSource/hurl.git"
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" \
    --filter=blob:none --sparse "$repo" "$work/hurl" >&2
  git -C "$work/hurl" sparse-checkout set --no-cone \
    '/docs/grammar.md' '/packages/hurl_core/src/parser/' >&2
  src="$work/hurl"
fi

parser="$src/packages/hurl_core/src/parser"
grammar="$src/docs/grammar.md"

for f in "$parser/query.rs" "$parser/filter.rs" "$parser/predicate.rs" "$parser/function.rs" "$parser/option.rs"; do
  if [ ! -f "$f" ]; then
    echo "gen-grammar-inventory: missing $f" >&2
    exit 1
  fi
done

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

# --- queries: try_literal("name", reader) with a lowercase-first identifier,
# excluding the certificate field literals (which embed a `"`, e.g. `Subject`
# matched via r#"Subject""#) since the identifier pattern already excludes them.
grep -oE 'try_literal\("[a-z][a-zA-Z0-9]*", *reader\)' "$parser/query.rs" \
  | sed -E 's/try_literal\("([a-z][a-zA-Z0-9]*)".*/queries\t\1/' >> "$tmp"

# --- filters: same shape, camelCase identifiers.
grep -oE 'try_literal\("[a-zA-Z][a-zA-Z0-9]*", *reader\)' "$parser/filter.rs" \
  | sed -E 's/try_literal\("([a-zA-Z][a-zA-Z0-9]*)".*/filters\t\1/' >> "$tmp"

# --- predicates: identifiers (excluding the "not" prefix modifier) + comparison operators.
grep -oE 'try_literal\("[a-zA-Z][a-zA-Z0-9]*", *reader\)' "$parser/predicate.rs" \
  | sed -E 's/try_literal\("([a-zA-Z][a-zA-Z0-9]*)".*/predicates\t\1/' \
  | grep -v -E $'^predicates\tnot$' >> "$tmp"
grep -oE 'try_literal\("[=!<>]+", *reader\)' "$parser/predicate.rs" \
  | sed -E 's/try_literal\("([=!<>]+)".*/predicates\t\1/' >> "$tmp"

# --- template functions: "name" => Ok(Function::...) match arms.
grep -oE '"[a-zA-Z][a-zA-Z0-9]*" => Ok\(Function::' "$parser/function.rs" \
  | sed -E 's/"([a-zA-Z][a-zA-Z0-9]*)".*/functions\t\1/' >> "$tmp"

# --- request [Options] section keys: "name" => option_xxx(reader)? match arms
# (the verbosity value arms "brief"/"verbose"/"debug" => Ok(OptionKind::...)
# have no "=> option_" call and are excluded by this pattern).
grep -oE '"[a-z0-9.-]+" => option_[a-z0-9_]+\(reader\)\?,' "$parser/option.rs" \
  | sed -E 's/"([a-z0-9.-]+)".*/options\t\1/' >> "$tmp"

sort -u "$tmp" > "$out"

# Cross-check against docs/grammar.md, best-effort, informational only: each
# individual "*-query"/"*-filter"/"*-predicate"/"*-function" rule sits on its
# own line, and the first grammar-literal in it is that rule's own keyword
# (nested value grammar, e.g. a filter's string argument, is never the first
# literal on the line).
if [ -f "$grammar" ]; then
  names="$(cut -f2 "$out")"
  while IFS= read -r line; do
    # Literal string split (not a regex) so operator content such as "<" or
    # "<=" cannot confuse the match the way a `<`-excluding char class would.
    name="$(printf '%s\n' "$line" | awk -F'grammar-literal">' 'NF>1{split($2,a,"</span>"); print a[1]; exit}')"
    if [ -n "$name" ] && ! grep -q -F -x "$name" <<< "$names"; then
      echo "gen-grammar-inventory: docs/grammar.md documents '$name' but the parser does not implement it (skipped)" >&2
    fi
  done < <(grep -E 'grammar-rule-id" id="[a-z0-9-]+-(query|filter|predicate|function)"' "$grammar")
fi

count="$(wc -l < "$out" | tr -d ' ')"
echo "gen-grammar-inventory: wrote $count entries to ${out#"$root"/}" >&2
