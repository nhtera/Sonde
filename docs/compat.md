# Hurl Compatibility

Sonde targets Hurl 8.0.1 files and CLI behavior. This page lists every known
difference. Phase 5 extends it with runtime and CLI gaps.

## Syntax (`sonde check`, `sonde fmt`, all commands that read files)

Measured against the vendored Hurl 8.0.1 test tree (`testdata/conformance/hurl`):

- All 272 valid `.hurl` files outside `tests_error_parser` parse and print back
  byte for byte (also with CRLF line endings); the two intentionally invalid
  ones (invalid UTF-8, bad method) are rejected with Hurl's message.
- All 46 invalid files of `tests_error_parser` are rejected (its 3 valid
  companions parse); for the 44 with an `.err` oracle, the error position and
  the rendered message match exactly.

Differences on inputs outside the test tree:

| Input | Hurl 8.0.1 | Sonde |
|---|---|---|
| Regex literal errors other than an invalid `{…}` repetition | Rust `regex` message | Go `regexp` message (same position) |
| Regexes valid in only one engine, e.g. `a{,3}`, `\d{2}{2}`, `a{1001}`, `[]-Z]` | accepted | rejected |
| … and the reverse, e.g. `[0-9[Query]` | rejected | accepted |
| XML body with DTD-declared entities | accepted (libxml2) | rejected: `invalid XML` |
| XML body with text or a comment before the root element | rejected | accepted |
| XML error position | character where libxml2 failed | last character read by Go's `encoding/xml` (same in all test files) |
| JSON nested deeper than 1000 levels | stack overflow | rejected: `nesting is deeper than 1000 levels` |
| File larger than 64 MiB | accepted | rejected: `Issue reading from FILE: file is larger than 64 MiB` |

Accepted as Hurl does: text after a placeholder's variable inside a string,
e.g. `{{a b}}` or `{{a}b}}`, is ignored (and kept by `sonde fmt`); XML
declarations with any encoding or version.

`sonde fmt` is not `hurlfmt`: it only normalizes horizontal whitespace and
line endings (see `docs/architecture.md`, CLI commands) and never reorders
sections. Files already formatted by `hurlfmt` are left unchanged.
