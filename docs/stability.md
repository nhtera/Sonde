# Stability

What v1 promises to keep working, and what it does not. "v1.x" below means
any `1.y.z` release; a change that breaks something this page covers waits
for `2.0.0`.

## `.hurl` compatibility

Sonde targets the Hurl 8.0.1 grammar and CLI behavior. [compat.md](compat.md)
(generated from `internal/docs/table.yaml`, `make docs`) is the complete,
maintained list of every known difference: parser edge cases, evaluation
differences, unsupported options, and CLI/env var/config gaps. It is
regenerated as gaps close, so it always reflects the current release, not a
point-in-time snapshot.

The [conformance harness](conformance.md) measures this against Hurl's own
vendored integration test suite and reports a semantic pass rate per lane
(exit code and stdout match) and a full-oracle pass rate (stderr too). The
**blocking lane** (plain HTTP only) is the one a release is judged on; run
`make conformance` for the current numbers on your machine, or see the CI
job's uploaded `conformance-results.json` for the last release build. A
regression in a blocking-lane script that was previously passing fails CI
(the manifest gate, [conformance.md](conformance.md#manifest-and-gate)).

Within v1.x, compatibility only improves: a difference `compat.md` lists as
a gap may be closed in a minor release; a passing case does not start
failing.

## CLI

Every command and flag in [cli/](cli/README.md) (generated from the cobra
command tree, `make docs`) is part of the v1 contract:

- **Additive only.** New commands, flags and `sonde.yaml` keys may appear in
  a minor release. An existing flag's name, type and default do not change,
  and a flag is never removed, within v1.x.
- **Deprecation, not removal.** If a flag or command needs to go away, it is
  marked deprecated (still works, prints a warning) for at least one minor
  release before v2.0.0 removes it. Nothing is silently deprecated: the
  change is called out in the release notes.

### Exit codes

Fixed: they are part of scripts and CI pipelines.

  | Code | Meaning |
  |---|---|
  | `0` | success |
  | `1` | CLI usage / option error |
  | `2` | input file parse error (also an unreadable input file) |
  | `3` | runtime error: connect, TLS, timeout, sandbox denial, unsupported option |
  | `4` | assert / contract failure |
  | `127` | undefined error (e.g. a report could not be written) — Hurl parity |
  | `130` | interrupted (Ctrl-C) — Sonde's own addition, on top of Hurl's set |

  Non-run subcommands have their own narrower set: `sonde check` uses `0`/`2`;
  `sonde fmt --check` adds `1` for unformatted files; `sonde import`/`sonde
  export` use `0`/`1` (usage) with `export curl` also using `2`/`3` (parse,
  render). See each command's page under [cli/](cli/README.md) and
  [architecture.md](architecture.md#5-contracts) for the exact table and the
  multi-file aggregation rule.

## JSON result schema

The schema `--json` and `--report-json` share is documented in
[report-json.md](report-json.md): a Hurl-8.0.1-compatible base (existing
Hurl tooling reads a Sonde result unchanged) plus a `sonde` key for
Sonde-only data (data rows, contract violations; later streams, gRPC).
Within v1.x, changes to this schema are additive only: new fields at the
base only to match the Hurl format, and Sonde-only data only under
`sonde`. Existing fields do not change type or meaning, and are not
removed.

## `sonde.yaml`

The project file schema — `environments`, `variables`, `variables_files`,
`secrets_files`, `defaults`, `openapi` — is documented in full in
[sonde-yaml.md](sonde-yaml.md). It is strict (an unknown key is an error),
so new keys can be added in a minor release without silently changing the
meaning of an existing file. An existing key's meaning and precedence do
not change within v1.x.

## Go API surface

The `engine` and `exchange` packages are the only public Go API; everything
under `internal/` is exactly that — internal, and may change in any release,
including a patch release, without notice. `engine` and `exchange` follow
semver from `v1.0.0`: a breaking change to an exported type, function or
method signature is a major version bump. This is checked in CI with
[`apidiff`](https://pkg.go.dev/golang.org/x/exp/cmd/apidiff) (`make
apicheck`) against the ref in `.api-baseline`, `v1.0.0`.

What the contract covers:

- `engine`: `Runner` and its methods (`RunFile`, `RunSource`, `RunAll`,
  `RenderCurl`, `Redact`, `HasSecrets`, `Close`), `Options`, `HTTPOptions`,
  `RunAllOptions`, `Job`, `Row`, the events (`Log`, `EntryStarted`,
  `EntryFinished`, `ContractEvaluated`), the results (`UnitResult`,
  `EntryResult`, `Call`, `Capture`, `Assert`, `Cookie`, `CurlEntry`), the
  opaque `Error` and `Value` types with their methods, `Pos`, `Span`,
  `Field`, `Redirect`, `ResponseValidator`, `NoContract`, `Violation`.
- `exchange`: `Request`, `Response`, `Header`, `Headers`, `Cookie`,
  `CookieAttribute`, `Timings`, `CertInfo`, `BodyError` and their methods.

Enumerations may grow in a minor release: `ErrorKind`, `ValueKind`,
`ViolationKind`, `LogLevel`, `exchange.BodyErrorKind`, and so may the set
of `Event` types. Switch on them with a `default` case. The text
of `Error()`, `Message()`, `Render()` and log events is for people and may
change in any release; match on `Kind()`, never on text.

## What this page does not cover

These are free to change in any release, including a patch release, and are
not part of the v1 contract:

- Exact log wording and progress-bar/spinner rendering on stderr (only the
  `--test` summary's documented format and per-file line are fixed, see
  [architecture.md](architecture.md)).
- Terminal color choices and ANSI sequence details.
- The HTML report's visual design (its data comes from the JSON schema
  above, which *is* covered).
- Anything under `internal/`, including `internal/report`'s Go types,
  `internal/config`, `internal/syntax`'s AST, and the language server
  protocol surface of `sonde lsp` beyond LSP itself (a standard protocol,
  not a Sonde contract).
- Benchmark numbers ([benchmarks.md](benchmarks.md)): a baseline for
  tracking regressions, not a guarantee.
