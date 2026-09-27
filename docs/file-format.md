# The Sonde file format

A Sonde file is a plain text file describing one or more HTTP requests, each
with its expected response, in the [Hurl](https://hurl.dev) language: a
request section, an optional response section (status, headers, asserts),
captures, and options — no markup, no scripting language.

```hurl
GET https://example.org/api/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
```

Sonde does not invent a new syntax for this: `internal/syntax` is a
line-for-line Go port of Hurl 8.0.1's own grammar (`hurl_core`), and
[compat.md](compat.md) tracks every input where the two parsers disagree.
The full grammar reference — sections, captures, asserts, filters,
predicates, templating, multiline strings — lives in Hurl's own docs:

- [hurl.dev/docs/hurl-file.html](https://hurl.dev/docs/hurl-file.html) —
  file structure
- [hurl.dev/docs/request.html](https://hurl.dev/docs/request.html) /
  [response.html](https://hurl.dev/docs/response.html) — request and
  response sections
- [hurl.dev/docs/asserting-response.html](https://hurl.dev/docs/asserting-response.html)
  — captures, asserts, queries, filters, predicates
- [hurl.dev/docs/templates.html](https://hurl.dev/docs/templates.html) —
  `{{variable}}` templating and functions

Sonde's own copy of that grammar is verified against Hurl 8.0.1's test suite
(`testdata/conformance/hurl`); differences, and every unsupported option, are
listed in [compat.md](compat.md).

## `.hurl` vs `.sonde`

The extension picks the grammar (`internal/syntax.DialectFor`):

| | `.hurl` | `.sonde` |
|---|---|---|
| Grammar | strict Hurl 8 | a superset of `.hurl` |
| Sonde extensions: `[SondeMessages]`, `sonde-stream-*` options, the `sondeStream` query | a parse error naming the construct | accepted ([guides/streaming.md](guides/streaming.md)) |
| Runs with Hurl itself | yes | no (Hurl rejects every Sonde construct at parse time) |
| `sonde fmt` | canonical layout, still valid Hurl | canonical layout |

A `.sonde` file that uses no extension behaves exactly like the same file
named `.hurl`. Use `.hurl` for files you want to keep interchangeable with
the `hurl` CLI or other Hurl tooling; use `.sonde` for Server-Sent Events
and WebSocket tests, and for files that are Sonde-only anyway. Input read
from standard input is parsed as `.hurl`. Every extension is prefixed
`sonde` so it never collides with syntax Hurl may add
([decisions/0004-streaming-protocols.md](decisions/0004-streaming-protocols.md)).
Both extensions are recognized by directory expansion (`sonde DIR`),
`sonde import`'s `--ext` flag, and the language server
([guides/editors.md](guides/editors.md)).

## What Sonde adds around the file

Nothing inside the file itself changes to enable these — they are CLI flags
or a `sonde.yaml` project file next to it:

| Addition | How | Docs |
|---|---|---|
| Environments (named variable/secret sets) | `sonde.yaml` + `--env` | [sonde-yaml.md](sonde-yaml.md) |
| OpenAPI contract checks | `--openapi` or `sonde.yaml` `openapi:` | [guides/openapi.md](guides/openapi.md) |
| Data-driven runs (once per CSV/JSON row) | `--data` | [guides/data-driven.md](guides/data-driven.md) |
| Importing from curl, Postman, OpenCollection, `.http`, OpenAPI | `sonde import` | [guides/import-export.md](guides/import-export.md) |
| Editor support (diagnostics, completion, hover, formatting) | `sonde lsp` | [guides/editors.md](guides/editors.md) |

See [architecture.md](architecture.md) §1 ("Sonde extras never change
`.hurl` files") for the design rule behind this table, and
[stability.md](stability.md) for what v1 promises to keep working.

## Canonical formatting

`sonde fmt` rewrites horizontal whitespace and line endings only: it never
reorders sections, never touches body content, and keeps blank lines and
comments. A file already laid out by Hurl's own `hurlfmt` is left unchanged.
`sonde fmt --check` lists files that are not in canonical form (exit `1`);
`sonde fmt --write` rewrites them in place. See
[cli/sonde_fmt.md](cli/sonde_fmt.md).
