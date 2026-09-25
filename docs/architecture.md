# Sonde Architecture & Cross-Cutting Contracts

> **Owner: this file.** The copy in the private implementation plan is historical.
> **Rule:** any pull request that adds or changes an exported symbol, CLI flag,
> exit code, config key or report field updates this file (and the owning doc)
> in the same pull request. Phase numbers refer to the implementation roadmap.

All paths are relative to the repository root.

## 1. Principles

1. **One engine, many frontends.** CLI now; later Wails v3 GUI (separate plan), `go test` embedding, LSP, MCP. All call the same `engine` package.
2. **Plain text is the source of truth.** `.hurl` = strict Hurl 8 grammar. `.sonde` = identical syntax in v1. Post-v1 protocol extensions (WS/SSE/gRPC) allowed **only** in `.sonde`, under a Sonde-specific prefix Hurl will not use — *confirmed in validation 2026-09-23*.
3. **Sonde extras never change `.hurl` files.** OpenAPI contracts, data-driven runs, environments = CLI flags / `sonde.yaml` only.
4. **Engine is a library.** No `os.Exit`, no printing, no package globals, no `init()` side effects. `context.Context` cancel everywhere. Typed events delivered serially. JSON-serializable, already-redacted results.
5. **Hurl HTTP semantics, explicit.** Body queries see decoded bytes (`rawbytes` = raw); redirects followed only on request, via a manual loop with curl's credential rules; curl-like defaults (`Accept: */*`, `User-Agent: sonde/<version>`).
6. **Pure Go, static binary.** `CGO_ENABLED=0`, `-trimpath`. No cgo dependencies in the CLI module.
7. **Secure by default.** TLS verification on; every file path coming from a request file goes through one sandboxed file-access layer; `--file-root` is CLI-only and `sonde.yaml` cannot change file access; secrets redacted inside the engine before anything leaves it; zero telemetry; no network access except requests the user wrote.
8. **Hurl-compatible CLI surface.** `sonde [options] FILE...` behaves like `hurl`, `sonde --test` like `hurl --test`; Hurl flag names, `HURL_*` env options, Hurl config file, exit codes — verified by running Hurl's own test scripts through a `hurl` → `sonde` shim.

## 2. Module & Package Map

Module: `github.com/nhtera/sonde` · `go 1.26` directive (supports Go 1.26 + 1.27) · toolchain `go1.27.x`.

| Package | Role | Allowed internal deps |
|---|---|---|
| `cmd/sonde` | `main` → `cli.Execute()` | `internal/cli` |
| `engine` (**public**) | `Runner`, `Unit`, `RunFile`, `RunSource`, `Options`, `HTTPOptions`, events, results, `ResponseValidator`, `Violation` | syntax, value, redact, template, query, filter, predicate, exchange, httpx, sandbox |
| `exchange` (**public**) | transport-neutral `Request`/`Response`/`Timings`/`CertInfo`/`Cookie` model | value |
| `internal/syntax` | reader, parser, AST, lossless printer, canonical formatter, diagnostics, dialect gate | — (leaf) |
| `internal/value` | typed `Value` (null/bool/int/bigint/float/string/bytes/list/object/date/nodeset) | — (leaf) |
| `internal/redact` | per-run, append-only, concurrency-safe secret registry + encoded-variant matching | — (leaf) |
| `internal/sandbox` | `os.Root`-based file access for all request-file paths | — (leaf) |
| `internal/template` | `{{ }}` rendering, functions (`newUuid`, `newDate` — Hurl 8 set only) | syntax, value |
| `internal/jsonpath` | RFC 9535 evaluator over `value.Value`, sorted-key traversal, Hurl unwrap rule (fork of `theory/jsonpath`, MIT) | value |
| `internal/query` | all Hurl queries over `exchange.Response` | syntax, value, exchange, jsonpath |
| `internal/filter` | all Hurl filters | syntax, value, jsonpath |
| `internal/predicate` | all Hurl predicates | syntax, value |
| `internal/httpx` | client/transport builder, options, manual redirect loop, timings, cookie jar, decompression | exchange, sandbox |
| `internal/report` | incremental terminal, JSON, JUnit, TAP, HTML renderers (consume redacted events/results) | engine |
| `internal/config` | `sonde.yaml`, Hurl config file, variables/secrets files, env vars, precedence, `sonde.yaml`/variables emitter | value, sandbox |
| `internal/dataset` | CSV / JSON-array rows for `--data` | value |
| `internal/openapi` | spec load (owns remote fetch), route match, `engine.ResponseValidator` impl, spec → AST generator | exchange, syntax, engine (interface only) |
| `internal/convert` | shared import writer + flags; curl/Postman/OpenCollection/`.http` importers; single curl renderer | syntax, exchange, config |
| `internal/docs` | single doc-data source (`table.yaml`) for compat.md, LSP hover | — |
| `internal/lsp` | language server | syntax, config, docs |
| `internal/cli` | cobra commands, Hurl-compatible root, flag → `engine.Options` mapping, output wiring | everything above |
| `internal/mock` (post-v1) | OpenAPI mock server | openapi |
| `internal/stream` (post-v1) | WebSocket + SSE | exchange, httpx |
| `internal/grpcx` (post-v1) | gRPC dynamic client | exchange |
| `internal/mcp` (post-v1) | MCP server | engine, syntax, config |
| `editors/vscode` | VS Code extension (TypeScript) — separate npm package | — |
| `testdata/conformance/hurl` | vendored Hurl 8 test tree + servers + requirements + manifest | — |

**Rules (enforced by `depguard` in `.golangci.yml`):** leaves import nothing internal; nothing imports `internal/cli`; `internal/report` imports only `engine` result/event types; `net/http` allowed only in `httpx`, `exchange` (`http.ParseSetCookie`), `stream`, `mock`, `openapi` (remote fetch, opt-in), `grpcx`; `_test.go` files exempt. Go source file names: `snake_case.go`; shell scripts: `kebab-case.sh`. One decision-record series: `docs/decisions/NNNN-*.md` (RFCs are decision records with status `proposed`).

## 3. Execution Flow

```mermaid
flowchart LR
  S[UnitSource: file × data row × repeat] --> RU[engine.Runner]
  C[config: vars, secrets, env] --> RU
  RU -->|per unit, isolated| P[syntax.Parse]
  P --> T[template.Render]
  T --> B[request builder + sandbox]
  B --> H[httpx: manual redirect loop]
  H -->|exchange.Response + Timings| E[evaluate]
  E --> Q[query → filter → predicate]
  E --> V[ResponseValidator optional]
  Q --> K[captures → vars + redact registry]
  RU -->|serial dispatcher, redacted| O[OnEvent: CLI renderer, incremental reports, GUI later]
  RU -->|after run, final secret union| X[run-level sinks: --curl, --cookie-jar, report finalize]
```

Per entry: render → build → send (retry loop wraps the whole entry) → implicit asserts (version, status, headers) → captures (new secrets registered) → explicit asserts → contract validation → emit buffered events (redacted). A unit stops at its first failing entry unless `--continue-on-error`. Units never cancel each other.

## 4. Public API Sketch (`engine`, `exchange`)

```go
// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

// Runner executes units; owns per-run state (secret registry, event dispatcher).
func NewRunner(opt Options) *Runner
func (r *Runner) Run(ctx context.Context, src UnitSource) (*RunSummary, error) // error = setup/config only
func (r *Runner) Close() error

// Convenience wrappers (single unit, own Runner).
func RunFile(ctx context.Context, path string, opt Options) (*UnitResult, error)
func RunSource(ctx context.Context, name string, src []byte, opt Options) (*UnitResult, error)

type UnitSource interface{ Next() (Unit, bool, error) } // lazy: files × rows × repeats
type Unit struct {
	Path   string
	Source []byte         // nil = read Path through sandbox
	Row    *DataRow       // nil = no data iteration
	Repeat int            // 1-based repeat index
}

type Options struct {
	Variables       map[string]any    // typed: string, int64, float64, bool, nil, []any, map[string]any
	Secrets         map[string]string // registered in the run's redact registry
	FileRoot        string            // CLI-only; default: dir of each file
	HTTP            HTTPOptions       // global defaults; per-entry [Options] override
	Validator       ResponseValidator // optional (OpenAPI contract); nil = off
	OnEvent         func(Event)       // called serially from one dispatcher goroutine; may block briefly
	Jobs            int               // parallel units; 0 = runtime.NumCPU()
	Retry           int               // -1 = unlimited
	RetryInterval   time.Duration
	Delay           time.Duration     // not applied to retries
	Repeat          int
	FromEntry, ToEntry int            // 1-based, 0 = unset
	NoAssert, ContinueOnError bool
}

type HTTPOptions struct { /* all Hurl request options: TLS, proxy, resolve, connect-to, protocols, timeouts, redirects, compression, netrc, limit-rate, max-filesize … (list owned by docs/compat.md) */ }

type ResponseValidator interface {
	ValidateResponse(ctx context.Context, req *exchange.Request, resp *exchange.Response) []Violation
}
type Violation struct{ SpecPointer, InstancePath, Message string }
```

Events (carry unit id, file, entry index, source line): `UnitStarted`, `EntryStarted`, `RequestSent`, `ResponseReceived` (+ `Timings`), `CaptureSet`, `AssertEvaluated`, `ContractEvaluated`, `EntryFinished`, `UnitFinished` (carries redacted `*UnitResult`). Events of an entry with `redact` captures are buffered until its captures ran. Per-unit parse/runtime errors live in `UnitResult`, never cancel other units. `RunSummary` holds counts + failures only (results are streamed, not accumulated).

API stability: pre-1.0 may change; from v1.0 `engine` + `exchange` follow semver, checked by `apidiff` in CI (Phase 10).

## 5. Contracts

### Exit codes (Hurl 8 + one documented addition)
| Code | Meaning |
|---|---|
| 0 | success |
| 1 | CLI usage / option error |
| 2 | input file parse error (also unreadable input file) |
| 3 | runtime error (connect, TLS, timeout, sandbox denial, unsupported option) |
| 4 | assert / contract failure |
| 127 | undefined error (e.g. report cannot be written) — Hurl parity |
| 130 | interrupted (Ctrl-C) — Sonde addition, documented |

Multi-unit aggregation: 1 and 127 abort; otherwise the most severe of 2 > 3 > 4 > 0 (verify against Hurl `main.rs` in Phase 5). Non-run subcommands: `fmt --check` → 1 if unformatted; `import` → 1 on unreadable input, 0 with warnings.

### Variable precedence (lowest → highest; Hurl-aligned)
1. `sonde.yaml` environment: `variables`, then `variables_files`
2. `HURL_VARIABLE_*`, then `SONDE_VARIABLE_*` env vars
3. `--variables-file` (in order given)
4. data row (`--data`)
5. `--variable`
6. entry `[Options] variable:` (entry-scoped)
7. captures during the run (unit-scoped)

Environment selection: `--env` > `SONDE_ENV` > `defaults.env`.
Secrets: `sonde.yaml` `secrets_files`, `HURL_SECRET_*`/`SONDE_SECRET_*`, `--secrets-file`, `--secret`, `--data-secret` columns, `redact` captures. Same secret name from two sources → error (Hurl message). Secret vs variable name clash → exit 1. Secrets shorter than 4 chars → warning.

Type inference for CLI/env/CSV values (Hurl-compatible): `true`/`false` → bool, `null` → null, integer → int, float → float, else string.

### Redaction
One registry per run (`internal/redact`): union of all sources incl. dynamic captures; values never removed. Matches raw, base64, URL-encoded and JSON-escaped forms. Applied inside the engine to events and results; run-level sinks (`--curl`, `--cookie-jar`, report finalization) written after the run with the final union.

### File access
Every path originating from a request file — `file,` bodies/parts, `output`, `cacert`, `cert`, `key`, `netrc-file`, `unix-socket`, cookie files — resolves through `internal/sandbox` (`os.Root`) under the file root. `--file-root` is CLI-only; `sonde.yaml` has no file-root key. `sonde.yaml` paths resolve relative to its directory and must stay inside it. The user's `~/.netrc` credentials are not sent when `connect-to`/`resolve`/`proxy` reroutes the host (unless an explicit flag allows it).

### HTTP semantics
Body queries (`body`, `bytes`, `sha256`, `jsonpath`, …) see decoded content; `rawbytes` sees raw; `compressed` only adds `Accept-Encoding` and decodes stdout. Redirects: manual loop, one RoundTrip per hop, per-hop timings, credentials forwarded only on same host + port + scheme unless `location-trusted`. Header names are canonicalized by Go (documented). `http2` on `http://` stays HTTP/1.1 (h2c upgrade unsupported). Decoded body cap 512 MiB default (`max-filesize` overrides).

### Unsupported features
Parser accepts the **full** Hurl 8 grammar. Anything the runtime does not implement → runtime error (exit 3) `option "aws-sigv4" is not supported by sonde yet`, tracked in `docs/compat.md`. Never silently ignore.

### JSON result contract
One schema for `--json` and `--report-json`: **Hurl-compatible base** (Hurl 8.0.1 JSON result shape, so Hurl's `--json` conformance tests and existing tooling work) with all Sonde-only data (contracts, iterations, streams, gRPC) under a top-level `sonde` key per object; additive changes only within a major version; documented in `docs/report-json.md`; covered by `docs/stability.md`. JUnit/TAP/HTML layouts follow Hurl's where its conformance tests compare them.

### `sonde.yaml`
Owner: `docs/sonde-yaml.md` (Phase 5) — the only place keys are defined; strict (unknown key → error); discovered per input file (nearest ancestor, cached per directory); listed in `docs/stability.md`.

## 6. Library Choices (licenses verified 2026-09-23)

| Need | Library | License |
|---|---|---|
| CLI | `github.com/spf13/cobra` | Apache-2.0 |
| JSONPath (RFC 9535) | internal fork of `github.com/theory/jsonpath` (sorted-key traversal) | MIT — attribution in NOTICE |
| XPath / HTML / XML | `github.com/antchfx/xpath`, `htmlquery`, `xmlquery` | MIT |
| Brotli / zstd | `github.com/andybalholm/brotli`, `github.com/klauspost/compress/zstd` | MIT / BSD-3 (verify via go-licenses) |
| HTTP/3 | `github.com/quic-go/quic-go/http3` | MIT |
| strftime (`format` filter) | `github.com/lestrrat-go/strftime` | MIT |
| JSON pretty/color | `github.com/tidwall/pretty` | MIT |
| YAML (`sonde.yaml`, OpenCollection) | `go.yaml.in/yaml/v3` (yaml/go-yaml) | Apache-2.0 / MIT |
| Parallelism | `golang.org/x/sync/semaphore` (no errgroup cancel-on-error) | BSD-3 |
| Public suffix / charsets / rate / term | `golang.org/x/net/publicsuffix`, `x/text`, `x/time/rate`, `x/term` | BSD-3 |
| OpenAPI (spike decides) | `github.com/pb33f/libopenapi` + `libopenapi-validator` **or** `github.com/getkin/kin-openapi` | MIT |
| Shell words (curl import) | `github.com/mattn/go-shellwords` | MIT (`google/shlex` archived — rejected) |
| LSP types | `go.lsp.dev/protocol` (go-language-server/protocol) | BSD-3 (`tliron/glsp` stale since 2025-06 — rejected) |
| WebSocket (post-v1) | `github.com/coder/websocket` | ISC |
| gRPC (post-v1) | `google.golang.org/grpc`, `github.com/bufbuild/protocompile` | Apache-2.0 |
| MCP (post-v1) | `github.com/modelcontextprotocol/go-sdk` | NOASSERTION on GitHub → verify before adopting |

CI gate: `go-licenses check ./... --allowed_licenses=Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC`.

## 7. Testing Strategy

| Layer | Technique | Where |
|---|---|---|
| Unit | table-driven, `-race` | every package |
| Golden | `testdata/**/*.golden`, `go test ./... -update` rewrites | syntax, report, convert, openapi |
| Property | lossless `Print(Parse(x)) == x`; `Format` idempotent | syntax |
| Determinism | `-count=100` on JSONPath, report ordering, mock generation | jsonpath, report, mock |
| Fuzz | `FuzzParse`, `FuzzCurlImport`, `FuzzHTTPFile`, `FuzzPostman`, `FuzzSSEParser` (post-v1) | CI smoke 30 s each; nightly 10 min |
| Integration | `httptest` servers (TLS, h2, redirects, cookies, gzip, proxy) | engine, httpx |
| Conformance | Hurl 8.0.1 test scripts run **unchanged** under bash with a `hurl` → `sonde` shim; oracles `.exit`/`.out`/`.out.pattern`/`.err`/`.err.pattern` (255 = skip); sequential; server lifecycle owned by `TestMain`; lanes **blocking** / extended (SSL, IPv6, unix, proxy) / network (skipped in CI) / timing (quarantine); metrics **semantic** (exit + stdout) and **full-oracle**; manifest + no-regression gate on blocking lane | `test/conformance` (build tag `conformance`) |
| Differential (local) | same file through real `hurl` and `sonde`, compare exit code + JSON via documented projection | `scripts/diff-hurl.sh` |
| Coverage | ≥85% for syntax, value, template, jsonpath, query, filter, predicate; ≥75% overall | CI report |

## 8. Performance

- Hard budgets (blocking tests, local benchmarks): parse 1 MB < 50 ms; LSP diagnostics on 1,000 lines < 50 ms.
- CI performance jobs are **report-only** (cold start, RSS, binary size, parallel speedup with a latency-injecting server); shared runners are too noisy to gate.
- Competitor comparisons (`hurl`, `xh`, `newman`, `bru`) via local `scripts/bench.sh`, published with methodology in `docs/benchmarks.md` — supports the "fast, lightweight" pitch.
- Binary size recorded per release; any dependency adding > 3 MB needs justification in its PR.

## 9. Security Model

Assets: secrets (tokens, passwords), local files, user's ambient credentials (`~/.netrc`), user's network position. **Trust model:** request files and `sonde.yaml` in a repository are untrusted input; CLI flags and the user's config dir are trusted.

| Threat | Control |
|---|---|
| Secret leakage (terminal, events, reports, `--curl`, `--cookie-jar`, LSP, MCP) | run-wide redact registry incl. dynamic captures and encoded variants; buffered events for `redact` entries; run-level sinks written after run; grep test over all sinks for CLI, env, data-row and dynamic secrets |
| Untrusted file reads/writes local files (`file,`, `output`, cert/key/netrc/socket paths) | `internal/sandbox` (`os.Root`) for all request-file paths; `--file-root` CLI-only; `sonde.yaml` cannot change file access, its own paths confined to its directory |
| Ambient credential forwarding | no `~/.netrc` credentials on rerouted hosts; redirect credential rule = same host+port+scheme |
| Env-var exfiltration via templates | no `getEnv` (not in Hurl 8); any future env access `.sonde`-only, allowlisted, auto-secret |
| Malicious import input | parsers fuzzed; no code execution (scripts → comments); size limits; output confined to `-o DIR` |
| Decompression bombs / huge bodies | decoded body cap (512 MiB default), stream limits |
| XSS in HTML report | `html/template`, bodies as escaped text, strict CSP, no remote assets |
| OpenAPI remote specs / `$ref` (SSRF, local file read) | single opt-in `--openapi-allow-remote`; fetching owned by `internal/openapi` |
| MCP agent misuse (post-v1) | `run` off by default, project-root sandbox, host allowlist, redaction, audit log |
| Supply chain | minimal deps, `govulncheck`, `go-licenses`, Actions pinned by SHA, conformance CI without secrets (`contents: read`), Python deps `--require-hashes`, extension lockfile + `npm audit`, signed releases + SBOM, publish tokens only in protected `release` environment |

## 10. GUI-Readiness Checklist (for the future Wails plan)

- [ ] `engine.Runner` with `Close()`, serial events, cancel, redacted JSON-serializable results (Phases 4–5)
- [ ] public `exchange` types → GUI can render requests/responses (Phase 3)
- [ ] lossless AST + formatter → GUI edit/save keeps comments (Phase 2)
- [ ] diagnostics with byte-offset spans → GUI inline errors (Phase 2, reused by LSP Phase 9)
- [ ] stable JSON result schema (Phase 5)
- [ ] `sonde.yaml` environments → GUI env switcher (Phase 5)
- Note: nested module `github.com/nhtera/sonde/gui` can import `github.com/nhtera/sonde/internal/...` (path-based rule, verified via gopls precedent) but internal packages are outside apidiff → GUI pins exact commits or needed APIs get promoted to public packages.
