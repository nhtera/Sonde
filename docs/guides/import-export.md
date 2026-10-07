# Import and export

`sonde import KIND INPUT` converts INPUT, written in another format, to
Sonde request files. This guide covers what every kind shares (the common
flags, and the writer's rules for naming files, handling collisions and
never touching an existing `sonde.yaml`), then each kind, then
`sonde export` (curl, JSON, HTML).

```sh
sonde import openapi petstore.yaml -o requests/
sonde import curl - -o requests/ < commands.sh
```

Imports never run anything: scripts in the input are kept as `#`
comments, and files the input names become `file,` references, never read.
An input file, or an environment file an importer reads, is limited to
64 MiB.

## Kinds

| Kind | Input | `--group` | Status |
|---|---|---|---|
| `openapi` | An OpenAPI 3 document, or Swagger 2.0 | `tag` (default), `path`, `flat` — see [openapi.md](openapi.md) | Available |
| `curl` | curl command lines, or a shell script holding them; `-` reads stdin | — (one file) | Available |
| `postman` | A Postman Collection v2.1 or v2.0 JSON file | `request` (default: one file per request, folders become directories), `folder` (one file per folder, its requests chained) | Available |
| `opencollection` | An OpenCollection 1.0 YAML file, a collection directory, or a zip of one | — (one file per request, folders become directories) | Available |
| `http` | A `.http` file (JetBrains HTTP Client or VS Code REST Client); `-` reads stdin | — (one file) | Available |

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

- Every path segment is sanitized to kebab-case: it is lowercased, any run
  of characters other than letters and digits becomes a single `-`, and
  leading/trailing `-` are trimmed. Letters outside ASCII are kept
  (`Élan Vital` → `élan-vital`). An empty segment (or an entire path
  that sanitizes to nothing) becomes `request`. A Windows-reserved device
  name (`con`, `prn`, `nul`, `com1`, ...) gets a `-file` suffix so the
  files stay portable. A segment longer than 100 bytes is cut, and ends
  with a short hash of the full name so that two long names stay
  distinct.
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

## Other files

An importer may write other files next to the request files: an
environment's secrets stub, `secrets/<environment>.secrets`, for example.
Their paths are already sanitized by the importer, so the `secrets_files`
entries of `sonde.yaml` name them exactly. A file the user goes on to fill in, such as a
secrets stub, is written `0o600` and, like `sonde.yaml`, never overwritten,
even with `--force`. The summary lists it as left untouched.

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

## curl

`sonde import curl INPUT` turns every curl command of INPUT into one entry
of a single file named after INPUT (`curl.hurl` for stdin), in order.
INPUT may be a single command or a pasted shell session:

- **Splitting:** a command starts at a `curl` word at the start of a line
  (a `$ ` prompt is dropped), or after `;` or `&&`. Pipes and redirects
  (`|`, `>`, `2>&1`) end a command, and other shell commands are ignored.
- **Quoting:** `'…'`, `"…"`, `$'…'` and `\` line continuations work as in
  the shell.
- **Shell variables:** `$NAME` and `${NAME}` outside single quotes become
  the Sonde variable `{{NAME}}`, with a warning to set it. Other
  expansions (`$(…)`, backticks, `${X:-y}`) are never run; they stay as
  text, with a warning.
- **Windows commands:** a command copied for Windows `cmd` (`^`
  continuations) is skipped, with a hint to copy it for bash.
- **Limit:** at most 10,000 commands.

Flags behave as in curl:

- **Data:** `-d`/`--data`, `--data-binary`, `--data-raw`,
  `--data-urlencode` and `--json` are joined with `&`, and imply `POST`.
  `-d` data is sent exactly as given, with
  `application/x-www-form-urlencoded` unless a `Content-Type` is set.
  `--json` sets `Content-Type` and `Accept` to `application/json`. `-G`
  appends the data to the URL instead.
- **Files:** `-d @file` and `--json @file`, when the only data, become a
  `file,` body. `--data-raw @x` is literal text. `-F name=@file` becomes
  a `[Multipart]` file, keeping its `;type=`; `--form-string` values are
  literal.
- **Headers:** `-H 'Name: value'` adds a header. `-H 'Name;'` sends it
  empty. `-H 'Content-Type:'` removes the content type `-d` would add.
- **Other flags:**
  - `-u` becomes the `user` option. A literal password gets a warning
    that it is written in plain text.
  - `-X`, `-I`, `-b` (`name=value` pairs), `-A` and `-e` map directly.
  - `-k`, `-L`, `--compressed`, `--http1.1`, `--http2`, `-x`,
    `--connect-timeout`, `-m`, `--cacert`, `--cert`, `--key`,
    `--resolve`, `--connect-to`, `--unix-socket`, `-o`, `--max-redirs`,
    `--limit-rate`, `--pinnedpubkey`, `-4`, `-6`, `--path-as-is` and
    `--location-trusted` map to the matching `[Options]`.
  - `--digest`, `--ntlm`, `--negotiate`, `--aws-sigv4`, `--http1.0` and
    `--http3` are kept as options with a warning: Sonde cannot send them
    yet, so those requests fail until it can.
  - `--retry N` becomes `retry: N` with `HTTP *` and a `status < 500`
    assert, so a 5xx response is retried as curl retries it; `-v` and
    `--verbose` become `verbose: true`.
- **Combined forms:** combined short flags (`-sSL`) and `--flag=value`
  both work.

Other output and diagnostic flags (`-s`, `-i`, `--fail`, …) are ignored.
Any other flag is skipped with a warning, together with its value.

## Postman

`sonde import postman collection.json -o DIR [--environment env.json]…`
reads a Collection v2.1 (or v2.0). See
[migrate-from-postman.md](migrate-from-postman.md) for the full mapping.

- **Auth:** inherited from the collection and its folders; `noauth` stops
  the inheritance. Basic, bearer and API key are converted. Digest, NTLM
  and AWS SigV4 are written as the matching `[Options]` with a warning:
  Sonde cannot send them yet, so those requests fail until it can. Other
  schemes are warned about and left out.
- **Bodies:** raw bodies, `[Form]`, `[Multipart]`, GraphQL (with its
  variables) and file bodies.
- **Variables:** collection and folder variables become the `collection`
  environment of `sonde.yaml`. Each `--environment` file becomes an
  environment of its own, which also holds the collection variables. The
  first one is the default.
- **Secrets:** a `secret` variable's value is never written, only its
  name, in a secrets stub (`secrets/<env>.secrets`).
- **Scripts:** kept as comments. `pm.response.to.have.status(N)` and
  `pm.expect(pm.response.code).to.eql(N)` become `HTTP N`.

## OpenCollection

`sonde import opencollection INPUT -o DIR` reads an OpenCollection 1.0
YAML collection: a single file, or a directory with `opencollection.yml`,
one `.yml` file per request, a `folder.yml` per folder and environments
under `environments/`, or a zip of that directory (read in place, never
extracted). Directory reads stay inside INPUT, symbolic links
included. A YAML file is limited to 16 MiB, and a directory to 256 MiB
and 10,000 files and directories. [decisions/0002-opencollection-mapping.md](../decisions/0002-opencollection-mapping.md)
pins the mapping. In short:

- Folders become directories, and auth and headers are inherited from
  folders.
- Environments become `sonde.yaml` environments, with `secret` variables
  as names in a secrets stub.
- Scripts, tests and actions are kept as comments; a simple status
  assertion (`res.status eq 200`) becomes `HTTP 200`.
- gRPC, WebSocket and script items are skipped.

## .http

`sonde import http INPUT -o DIR [--env-file http-client.env.json]…` reads
a JetBrains HTTP Client or VS Code REST Client file. All of its requests
become one file named after INPUT (`requests.hurl` for stdin).

- **Separators and names:** `###` separators and `# @name` names.
- **Request line:** `METHOD URL`, or a bare URL for a `GET`. JetBrains'
  indented `?a=1` / `&b=2` continuation lines are joined to the URL.
- **Bodies:** JSON, `[Form]` for form-encoded bodies, `[Multipart]` for
  simple multipart bodies, and text.
- **File bodies:** `< file` becomes a `file,` body. So does `<@ file`,
  with a warning: the source tool fills in variables inside that file,
  Sonde does not.
- **Output:** `>>! file` becomes the `output` option. So does `>> file`,
  with a warning: JetBrains keeps an existing file there, Sonde
  overwrites it.
- **Timeouts:** `# @timeout` and `# @connection-timeout` (seconds by
  default, or `ms`/`s`/`m`) become `max-time` and `connect-timeout`.
  JetBrains' `@timeout` is an idle timeout, so `max-time` is only an
  approximation.
- **Scripts:** `< {% %}`, `> {% %}` and `> script.js` are kept as
  comments.
- **File variables** (`@name = value`) become the `default` environment.
  `sonde.yaml` values are literal, so a file variable that refers to
  another one (`@api = {{host}}/v1`) is resolved when importing.
- **Environment files:** each `--env-file` environment becomes a
  `sonde.yaml` environment that also holds the file variables, and
  `$shared` values apply to all of them. A sibling
  `http-client.private.env.json` contributes secret names only, in a stub;
  passing a private file itself to `--env-file` is refused.

### Dynamic variables

Postman, OpenCollection and `.http` imports convert `{{$…}}` variables the
same way:

| Source | Sonde |
|---|---|
| `{{$uuid}}`, `{{$guid}}`, `{{$randomUUID}}`, `{{$random.uuid}}` | `{{newUuid}}` |
| `{{$isoTimestamp}}` | `{{newDate}}` |
| any other, e.g. `{{$timestamp}}`, `{{$randomInt}}`, `{{$processEnv HOME}}` | a variable of the same name without `$` (`{{timestamp}}`), with a warning |

A variable name with characters Sonde does not allow gets `_` instead
(`{{api.host}}` → `{{api_host}}`), in requests and in `sonde.yaml` alike.
A REST Client request variable (`{{login.response.body.$.token}}`) is
kept as a plain variable, with a warning to use a capture instead.

## Exporting to curl

`sonde export curl FILE… [--entry N]` prints each entry's curl command,
one per line, without sending anything. The commands are the same as
`--curl` writes for a run. Variables resolve as for a run of FILE:
`--variable`, `--variables-file`, `--secret`, `--secrets-file` and the
`sonde.yaml` environment (`--env`, `--config`). A variable nothing
defines, typically one an earlier entry would capture, stays `{{name}}`
in the output, with a warning on stderr. Secrets are redacted as in every
output, and no flag reveals them. An entry that `[Options]` skips
(`skip: true`, `repeat: 0`) is left out.

## Exporting the syntax tree: JSON and HTML

`sonde export json [FILE…]` prints each `.hurl` file's syntax tree as one
JSON document per file, and `sonde export html [--standalone] [FILE…]` prints
it as syntax-highlighted HTML: a `<pre>` block with one `<span class="…">`
per token, or with `--standalone` a complete page with its stylesheet. Both
read standard input when no FILE is given, write to `-o FILE` instead of
stdout, and print what `hurlfmt --out json` / `--out html` print (checked on
its fixtures and on every file of the conformance suite; the known
differences are in [compat.md](../compat.md)). `.sonde` files are refused. A file that does not parse is reported on
stderr, the others are still exported, and the command exits with `2`.

```sh
sonde export json api.hurl | jq '.entries[].request.url'
sonde export html --standalone api.hurl -o api.html
```
