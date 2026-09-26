# Import and export

`sonde import KIND INPUT` converts INPUT, written in another format, to
Sonde request files. Each kind understands its own input; this guide covers
what every kind shares: the common flags and the writer's rules for naming
files, handling collisions and never touching an existing `sonde.yaml`.

```sh
sonde import openapi petstore.yaml -o requests/
```

## Kinds

| Kind | Input | `--group` | Status |
|---|---|---|---|
| `openapi` | An OpenAPI 3 document | `tag` (default), `path`, `flat` — see [guides/openapi.md](openapi.md) | Available |
| `curl` | A `curl` command line | — | Phase 8 |
| `postman` | A Postman collection | — | Phase 8 |
| `opencollection` | An OpenCollection file | — | Phase 8 |
| `http` | A `.http` file | — | Phase 8 |

`sonde import` with no kind, or an unrecognized one, is a usage error
(exit 1) listing the kinds actually registered in the binary.

## Common flags

Every kind accepts:

- `-o, --output DIR` (required): where generated files are written.
- `--ext hurl|sonde` (default `hurl`): the extension generated files get.
- `--force`: allow overwriting files that already exist in `DIR`. It never
  applies to `DIR/sonde.yaml` (see below).
- `--dry-run`: print the plan (files that would be written, and any
  warnings) and write nothing.

A kind may add its own flags on top of these (`--group` for `openapi`, for
example).

## Naming

Each generated file gets a path (without extension) from the importer —
`pets/list-pets` for an OpenAPI `GET /pets` operation grouped by tag, say.
The writer turns that into a real path under `DIR`:

- Every path segment is sanitized to kebab-case ASCII: it is lowercased,
  any run of characters outside `[a-z0-9]` becomes a single `-`, and
  leading/trailing `-` are trimmed. An empty segment (or an entire path
  that sanitizes to nothing) becomes `request`. A Windows-reserved device
  name (`con`, `prn`, `nul`, `com1`, ...) gets a `-file` suffix so the
  files stay portable.
- `.` and `..` segments are dropped outright — they can never appear in
  the final path, so a generated path can never climb outside `DIR`.
- If two generated files sanitize to the same path, the writer appends
  `-2`, `-3`, ... before the extension to keep them distinct, in the order
  the importer produced them.
- The extension (`--ext`) is appended last.

Every write is confined inside `DIR`, including through a symbolic link
placed there: a path that would escape `DIR` fails the import rather than
writing outside it.

## Collisions, `--force` and `--dry-run`

Before writing anything, the writer checks whether any planned file
already exists in `DIR`:

- Without `--force`, any conflict fails the whole import (exit 1) and
  lists every conflicting file; nothing is written, including files that
  did not conflict.
- With `--force`, an existing generated file is overwritten. `DIR` itself
  is created if it doesn't exist yet.
- `--dry-run` computes the same plan (including the conflict check) and
  prints it, but writes nothing whether or not `--force` is given.

Each file is written atomically (a temporary file in the same directory,
renamed into place), so a reader never observes a partially written file.
Directories are created `0o755`, files `0o644`.

## `sonde.yaml`

An importer may propose a `sonde.yaml` skeleton alongside the request
files (`openapi.spec` pointing at the source spec, for example). The
writer creates `DIR/sonde.yaml` only if it does not already exist, and
says so in its summary either way. **`--force` never overwrites an
existing `sonde.yaml`** — a project file is something a user goes on to
edit by hand, and re-running an import must not clobber those edits. To
regenerate it, remove or rename the existing file first.

## Summary

After a run (or a `--dry-run`), Sonde prints a summary to stderr: the
files written (or planned) and how many requests they hold, any input
items the importer chose to skip and why, and any warnings, grouped by
kind. Order is deterministic (paths, skipped items and warnings are all
sorted), so the output diffs cleanly between runs of the same input.

Warnings never fail the import: a `sonde import` run exits 0 whether or
not it produced warnings, unless a flag or the input itself was invalid,
or a write conflicted without `--force` (both exit 1).
