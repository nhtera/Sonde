#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Self-test for check-license-headers.sh: a file with the header passes,
# a file without it fails.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/check-license-headers.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf '// Copyright 2026 The Sonde Authors\n// SPDX-License-Identifier: Apache-2.0\n\npackage x\n' > "$tmp/good.go"
printf 'package x\n' > "$tmp/bad.go"

printf '// Copyright 2026 The Sonde Authors\n// SPDX-License-Identifier: Apache-2.0\n\nexport {};\n' > "$tmp/good.ts"
printf '/* Copyright 2026 The Sonde Authors */\n/* SPDX-License-Identifier: Apache-2.0 */\n' > "$tmp/good.css"
printf 'export {};\n' > "$tmp/bad.tsx"

for f in good.go good.ts good.css; do
  "$script" "$tmp/$f" > /dev/null || { echo "FAIL: header present in $f but check failed" >&2; exit 1; }
done
for f in bad.go bad.tsx; do
  if "$script" "$tmp/$f" > /dev/null 2>&1; then
    echo "FAIL: header missing in $f but check passed" >&2
    exit 1
  fi
done

# Without arguments the check lists the repository's Go files, the
# desktop frontend sources and the website sources.
repo="$tmp/repo"
mkdir -p "$repo/desktop/frontend/src" "$repo/editors" "$repo/site/src" "$repo/site/scripts"
cp "$tmp/good.go" "$repo/main.go"
cp "$tmp/good.ts" "$repo/desktop/frontend/src/ok.ts"
cp "$tmp/good.ts" "$repo/site/src/ok.tsx"
printf 'export {};\n' > "$repo/site/src/routeTree.gen.ts" # generated: not checked
printf 'export {};\n' > "$repo/editors/elsewhere.ts" # outside desktop/: not checked
git -C "$repo" init -q
(cd "$repo" && "$script" > /dev/null) || { echo "FAIL: clean repository failed the check" >&2; exit 1; }
# A tracked file deleted but not yet staged is skipped.
git -C "$repo" add main.go
rm "$repo/main.go"
cp "$tmp/good.go" "$repo/other.go"
(cd "$repo" && "$script" > /dev/null) || { echo "FAIL: a deleted tracked file failed the check" >&2; exit 1; }
cp "$tmp/bad.tsx" "$repo/desktop/frontend/src/bad.tsx"
if (cd "$repo" && "$script" > /dev/null 2>&1); then
  echo "FAIL: desktop/**/*.tsx without a header passed" >&2
  exit 1
fi
rm "$repo/desktop/frontend/src/bad.tsx"
cp "$tmp/bad.tsx" "$repo/site/scripts/bad.mjs"
if (cd "$repo" && "$script" > /dev/null 2>&1); then
  echo "FAIL: site/**/*.mjs without a header passed" >&2
  exit 1
fi
echo "check-license-headers self-test ok"
