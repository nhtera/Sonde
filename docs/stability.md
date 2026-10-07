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

## `.sonde` extensions

`.sonde` files accept everything `.hurl` does, plus Sonde's own constructs
(`[SondeMessages]`, `[SondeGrpc]`, the `sonde-stream-*` options, the
`sondeStream` and `sondeGrpc` queries:
[guides/streaming.md](guides/streaming.md),
[guides/grpc.md](guides/grpc.md),
[decisions/0004-streaming-protocols.md](decisions/0004-streaming-protocols.md),
[decisions/0005-grpc.md](decisions/0005-grpc.md)).
They are part of the v1 contract like the CLI: new constructs, new
`[SondeGrpc]` keys and new query fields may appear in a minor release; an
existing one keeps its name, meaning and defaults within v1.x. That includes
the gRPC JSON mapping and the rule that an unchecked status other than OK
fails the entry. A `.hurl` file that uses one
is a parse error, and stays one.

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
  render); `sonde mock` uses `0` (stopped by SIGTERM), `1` (usage or
  unloadable spec), `3` (address cannot be bound) and `130` (Ctrl-C);
  `sonde mcp` uses `0` (the client closed the connection), `1` (usage), `3`
  (the server failed) and `130` (Ctrl-C). See
  each command's page under [cli/](cli/README.md) and
  [architecture.md](architecture.md#5-contracts) for the exact table and
  the multi-file aggregation rule.

## MCP tools

The tools of `sonde mcp` ([guides/mcp.md](guides/mcp.md)) are part of the v1
contract like the CLI: a tool keeps its name, its arguments and the meaning
of its output fields within v1.x. New tools, optional arguments and output
fields may appear in a minor release. The `result` of `sonde_run` follows
the JSON result schema below.

## JSON result schema

The schema `--json` and `--report-json` share is documented in
[report-json.md](report-json.md): a Hurl-8.0.1-compatible base (existing
Hurl tooling reads a Sonde result unchanged) plus a `sonde` key for
Sonde-only data (data rows, contract violations, streams, gRPC status).
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
  `EntryFinished`, `ContractEvaluated`, `MessageSent`, `MessageReceived`),
  the results (`UnitResult`,
  `EntryResult`, `Call`, `Capture`, `Assert`, `Cookie`, `CurlEntry`), the
  opaque `Error` and `Value` types with their methods, `Pos`, `Span`,
  `Field`, `Redirect`, `ResponseValidator`, `NoContract`, `Violation`.
- `exchange`: `Request`, `Response`, `Header`, `Headers`, `Cookie`,
  `CookieAttribute`, `Timings`, `CertInfo`, `BodyError`, `Stream`,
  `Message`, `Direction`, `GRPCStatus` and their methods, and the
  `Protocol*` and `Stop*` constants.

Enumerations may grow in a minor release: `ErrorKind`, `ValueKind`,
`ViolationKind`, `LogLevel`, `exchange.BodyErrorKind`, the stream protocols
and stop reasons, and so may the set of `Event` types. Switch on them with a `default` case. The text
of `Error()`, `Message()`, `Render()` and log events is for people and may
change in any release; match on `Kind()`, never on text.

`Options.StdoutBody` is experimental: its signature may change in a minor
release.

## Sonde Desktop

[Sonde Desktop](desktop.md) is not covered by the v1 contract above. It is a
separate program, a nested Go module in `desktop/`, with its own version
(`desktop/frontend/package.json`) released by `desktop/vX.Y.Z` tags,
independent of the CLI's `vX.Y.Z` tags
([release.md](release.md#desktop-release)).

- It depends on `internal/`, which this page leaves free to change, and on a
  pinned beta of Wails v3. It is built against the repository's own
  checkout, so a change under `internal/` can require a matching change in
  `desktop/` in the same commit. That is deliberate:
  [decisions/0007-desktop-module.md](decisions/0007-desktop-module.md).
- The Go API contract (`engine`, `exchange`, `make apicheck`) does not
  cover the desktop module: it exports no API, and `apidiff` never sees its
  code.
- Its settings, history and kept-jar files, its flags (`--root`, `--data`,
  `--perf-trace`, `--perf-tour`, and server mode's `--port` and `--open`),
  its keyboard shortcuts and its UI may change in any desktop release.
- What it relies on from the CLI's contract still holds: the same
  `.hurl`/`.sonde` grammar, `sonde.yaml`, variable precedence, flags
  and environment variables, and the JSON result shape of its exports.

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
- Sonde Desktop and its server mode, as above.
- Benchmark numbers ([benchmarks.md](benchmarks.md)): a baseline for
  tracking regressions, not a guarantee.
