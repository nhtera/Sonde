# Getting started

This walks through installing Sonde, writing a first request file, running
it, adding asserts and captures, running in test mode with reports, and
selecting variables through `sonde.yaml` environments. Every command below
was run against [httpbin.org](https://httpbin.org) while writing this guide.

## Install

```sh
go install github.com/nhtera/sonde/cmd/sonde@latest
```

See [README.md](../README.md#install) for Homebrew, Scoop and release
archive options. Check it worked:

```sh
sonde version
```

## Your first file

Sonde runs `.hurl` files (the [Hurl](https://hurl.dev) format) and `.sonde`
files (same syntax; see [file-format.md](file-format.md)). Create
`health.hurl`:

```hurl
GET https://httpbin.org/status/200
HTTP 200
```

## Run it

```sh
sonde health.hurl
```

Nothing prints: a `200` with an empty body has nothing to show, and a
passing run exits `0`. Use `-i` to see the response headers, or `--json` for
a machine-readable result:

```sh
sonde -i health.hurl
```

A failing assert exits `4`; a connection error exits `3` (see
[stability.md](stability.md#exit-codes) for the full table).

## Asserts and captures

An entry can assert on the response and capture values into variables the
rest of the file can use. `uuid.hurl`:

```hurl
GET https://httpbin.org/uuid
HTTP 200
[Captures]
id: jsonpath "$.uuid"
[Asserts]
jsonpath "$.uuid" matches /^[0-9a-f-]{36}$/

GET https://httpbin.org/anything/{{id}}
HTTP 200
[Asserts]
jsonpath "$.url" contains "{{id}}"
```

The first entry captures the response's `uuid` field into `{{id}}`; the
second entry reuses it in its URL and asserts it round-tripped. Queries,
filters and predicates are listed in [compat.md](compat.md).

```sh
sonde uuid.hurl
```

## Test mode and reports

`--test` (or `sonde test`) runs files in parallel, prints a per-file result
line, and ends with a summary instead of response bodies:

```sh
sonde --test uuid.hurl
```

Add `--report-html DIR`, `--report-junit FILE`, `--report-tap FILE` or
`--report-json DIR` to keep a report; reports accumulate across separate
invocations instead of being overwritten, so a CI job that runs several
`sonde --test` steps can write to the same report:

```sh
sonde --test --report-html reports uuid.hurl
open reports/index.html   # or xdg-open on Linux
```

The JSON result schema (`--json` and `--report-json`) is documented in
[report-json.md](report-json.md). For running files from `go test` instead
of the CLI, see [guides/go-test.md](guides/go-test.md); for CI, see
[guides/ci-github-actions.md](guides/ci-github-actions.md).

## Environments: `sonde.yaml`

A `sonde.yaml` next to your files (or in a parent directory) selects
variables per environment, so the same file runs against different targets
without editing it. Create `sonde.yaml`:

```yaml
version: 1

environments:
  demo:
    variables:
      base_url: https://httpbin.org

defaults:
  env: demo
```

And `status.hurl` in the same directory:

```hurl
GET {{base_url}}/status/200
HTTP 200
```

```sh
sonde status.hurl          # defaults.env picks "demo"
sonde --env demo status.hurl   # same, explicit
```

`sonde.yaml` variables are the lowest-precedence source: a `--variable`,
`--variables-file`, data row or capture on the command line always wins.
Secrets work the same way, through `secrets_files`. Full schema and
precedence rules: [sonde-yaml.md](sonde-yaml.md).

## Where next

- [file-format.md](file-format.md) — `.hurl` vs `.sonde`, and what Sonde adds
- [compat.md](compat.md) — every query, filter, predicate, function, option and flag
- [guides/data-driven.md](guides/data-driven.md) — running a file once per row of a CSV or JSON file
- [guides/openapi.md](guides/openapi.md) — validating responses against an OpenAPI contract
- [guides/import-export.md](guides/import-export.md) — importing curl commands, Postman, OpenCollection, `.http` files and OpenAPI specs
- [guides/migrate-from-hurl.md](guides/migrate-from-hurl.md) — switching an existing Hurl project
- [guides/editors.md](guides/editors.md) — `sonde lsp`: VS Code, Neovim and other editors
- [cli/README.md](cli/README.md) — every command and flag
- [stability.md](stability.md) — what v1 promises to keep working
