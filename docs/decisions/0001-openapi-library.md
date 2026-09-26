# 0001: OpenAPI response-validation library

Status: Accepted. Date: 2026-09-26.

## Context

Phase 7 needs a library to validate HTTP responses against an OpenAPI 3.0/3.1
document: given a path template, method, and a captured response (status,
headers, body), report schema/header/status/content-type violations with
enough location detail for a useful CLI report. Sonde will keep its own
request router (server base-path stripping, `--openapi-server` override,
literal-over-template precedence); the library only needs a "validate this
response against this known operation" entry point. Remote `$ref` fetching
must be off by default (untrusted specs, sandboxed transport). Must be pure
Go/no cgo, and only Apache-2.0/MIT/BSD-2/BSD-3/ISC transitive licenses.

Two candidates: `github.com/getkin/kin-openapi` (openapi3 + openapi3filter)
and `github.com/pb33f/libopenapi` + `github.com/pb33f/libopenapi-validator`.

## Measurements

Spike code: see report `plans/reports/ak-engineer:researcher-260926-1542-openapi-library-spike.md`.
Fixtures: `testdata/openapi/{petstore-3.0,petstore-3.1,edge-cases-3.1}.yaml`.
Large spec: github/rest-api-description `api.github.com.yaml` (9.9 MB, 810 paths), scratchpad-only.

| Dimension | kin-openapi v0.149.0 | libopenapi v0.40.1 + validator v0.14.0 |
|---|---|---|
| 3.1 correctness (`type:[string,null]`, `const`) | pass | pass |
| Message quality | verbose text w/ embedded schema+value dump; `Reason` field for short text | separate `Message`/`Reason`/`HowToFix` fields, cleaner for structured output |
| Location info | `SchemaError.JSONPointer()` (instance path) + `SchemaField` (keyword name only) | `SchemaValidationFailure.KeywordLocation` (full JSON pointer into schema) + `InstancePath` + `FieldPath` (JSONPath) + `SpecLine`/`SpecCol` |
| GH spec load | `LoadFromData` 923ms; `doc.Validate()` fails twice (bad example, then "conflicting paths") on real spec, must skip/relax it | `NewDocument`+`BuildV3Model` 284ms, succeeds cleanly; but `validator.NewValidator(doc)` (schema pre-compile) takes **~50s** regardless of options |
| Peak memory (whole spike process) | heap delta 46.7 MB (doc load) | heap delta 203.8 MB (doc build); process RSS 1.47 GB incl. the 50s validator build |
| Per-response overhead (10k loop, small spec) | 2.9 µs/call | 5.2 µs/call |
| Thread safety | shared `*openapi3.T`+`Route` across 50 goroutines, `-race` clean | shared `Validator` across 50 goroutines, `-race` clean |
| Swagger 2.0 | `openapi2.T` + `openapi2conv.ToV3` (works); plain `openapi3.Loader` silently mis-parses a v2 doc (footgun) | `BuildV2Model()` native; `BuildV3Model()` on a v2 doc errors with a clear message |
| Remote `$ref` off by default | yes, fails fast at `LoadFromData` with a clear error | yes, fails at `BuildV3Model` with a clear error (plus a JSON warning log) |
| Own-router hook ("validate against known op") | build `routers.Route{Path, PathItem, Method, Operation}` by hand, pass to `openapi3filter.ValidateResponse` | `responses.ResponseBodyValidator.ValidateResponseBodyWithPathItem(req, resp, pathItem, pathTemplate)` |
| Transitive deps actually compiled in | 5 (all allowed licenses) | 11 (all allowed licenses) |
| License (lib itself) | MIT | MIT |
| cgo | none | none |
| Last release / commit | v0.149.0 (2026-08-28) / commit 2026-09-20 | v0.40.1 (2026-09-25) / commit 2026-09-25 |
| GitHub stars (adoption proxy) | 3297 | 875 |

## Decision

Use **kin-openapi**.

The deciding factor is the ~50-second, option-independent cost of
`libopenapi-validator.NewValidator()` on a GitHub-scale spec (810 paths):
confirmed reproducible across two separate runs and independent of
`WithFormatAssertions()`. For a CLI where `sonde run` is a single-shot
invocation, a 50s pause before the first request even fires is unacceptable
whenever a user points Sonde at a large third-party spec (GitHub's is used
here as a stress case, not a hypothetical). kin-openapi has no equivalent
monolithic pre-compile step; per-call overhead is already sub-microsecond-
scale and dependency footprint is under half of libopenapi's (5 vs 11
compiled modules). Its weaker location info (no schema-keyword JSON pointer,
only instance path + keyword name) and noisier default error text are
acceptable trade-offs Sonde's own report formatter can compensate for.
libopenapi/libopenapi-validator's richer `KeywordLocation`/`InstancePath`
pair and cleaner structured errors were attractive, but the compile-time
blowup and smaller community (875 vs 3297 stars, single-maintainer-led,
faster-moving 0.x API) make it the riskier pick for this project.

## Consequences

- Sonde must skip/relax `doc.Validate()` for third-party specs (it rejects
  the real GitHub spec on invalid examples and templated-path ambiguity that
  are not, in practice, invalid); only load + build routes, and validate
  responses directly.
- Swagger 2.0 input must go through `openapi2.T` + `openapi2conv.ToV3`, never
  the plain `openapi3.Loader` (which silently mis-parses v2 docs).
- Sonde's own report formatter must synthesize a schema-location string
  itself (kin-openapi gives keyword name + instance path, not a schema JSON
  pointer), likely by re-deriving it from `SchemaError.Schema`/`SchemaField`
  or by walking the operation's response schema alongside the instance path.
- Both libraries are pre-1.0; pin exact kin-openapi version and re-run this
  spike's validation matrix on any minor bump before upgrading.
- Revisit if kin-openapi's `doc.Validate()` strictness or its lack of a
  schema-keyword JSON pointer becomes a recurring complaint; libopenapi's
  compile-time cost might be avoidable via lazy/partial validator
  construction not explored in this spike.
