#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Fails if a source file lacks "SPDX-License-Identifier: Apache-2.0" in its
# first 3 lines. With arguments, checks those files; otherwise checks every
# tracked or untracked-but-not-ignored .go file in the repository, every
# .ts, .tsx, .css and .grammar file under desktop/, and every .ts, .tsx,
# .css and .mjs file under site/ (generated files excluded).
set -euo pipefail

if [ "$#" -gt 0 ]; then
  files=("$@")
else
  # List from the repo root, NUL-delimited so any file name survives; a git
  # failure or an empty list is an error, never a silent pass.
  root="$(git rev-parse --show-toplevel)"
  listing="$(mktemp)"
  trap 'rm -f "$listing"' EXIT
  git -C "$root" ls-files -z --cached --others --exclude-standard -- '*.go' \
    'desktop/*.ts' 'desktop/*.tsx' 'desktop/*.css' 'desktop/*.grammar' \
    'site/*.ts' 'site/*.tsx' 'site/*.css' 'site/*.mjs' \
    ':(exclude)site/src/routeTree.gen.ts' ':(exclude)site/.source/*' \
    ':(exclude)site/worker-configuration.d.ts' > "$listing"
  files=()
  while IFS= read -r -d '' f; do files+=("$root/$f"); done < "$listing"
fi

if [ "${#files[@]}" -eq 0 ]; then
  echo "no source files to check" >&2
  exit 1
fi

missing=0
for f in "${files[@]}"; do
  # A tracked file deleted in the working tree (not yet staged) is gone.
  [ -e "$f" ] || continue
  if ! head -n 3 "$f" | grep -q 'SPDX-License-Identifier: Apache-2.0'; then
    echo "missing SPDX header: $f" >&2
    missing=1
  fi
done

if [ "$missing" -ne 0 ]; then
  cat >&2 <<'MSG'
add this header to the files above (CSS: /* ... */ comments):
// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0
MSG
  exit 1
fi
echo "license headers ok (${#files[@]} files)"
