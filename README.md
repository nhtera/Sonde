# Sonde

![status: stable](https://img.shields.io/badge/status-stable-brightgreen)

**Sonde** (pronounced "sond") is a fast, single-binary CLI that runs and
tests HTTP requests written in plain text. It reads [Hurl](https://hurl.dev)
files (`.hurl`, and `.sonde` for the same syntax) and adds OpenAPI contract
checks, data-driven runs, `sonde.yaml` environments, importers from curl,
Postman, Bruno and `.http` files, and editor support (`sonde lsp`).

```hurl
GET https://httpbin.org/status/200
HTTP 200
```

```sh
sonde --test health.hurl
```

## Install

**Homebrew** (macOS/Linux):

```sh
brew install nhtera/tap/sonde
```

**Scoop** (Windows):

```sh
scoop bucket add nhtera https://github.com/nhtera/scoop-bucket
scoop install sonde
```

**Go:**

```sh
go install github.com/nhtera/sonde/cmd/sonde@latest
```

**Release archives:** signed, checksummed binaries for macOS, Linux and
Windows are on the [releases page](https://github.com/nhtera/sonde/releases).
Every release ships a `checksums.txt`, a `checksums.txt.sigstore.json`
cosign bundle and an SBOM; see [docs/release.md](docs/release.md#verifying-signatures)
to verify one.

## 30-second tour

Write a request file:

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

Run it, in test mode, with an HTML report:

```sh
sonde --test --report-html reports uuid.hurl
```

Point the same file at a different target without editing it, with a
`sonde.yaml` next to it:

```yaml
version: 1
environments:
  staging:
    variables:
      base_url: https://staging.example.internal
defaults:
  env: staging
```

Or start from what you already have:

```sh
sonde import postman collection.json -o requests/
sonde import curl commands.sh -o requests/
sonde import openapi petstore.yaml -o requests/
```

Full walkthrough: [docs/getting-started.md](docs/getting-started.md).

## Features

- **Hurl-compatible.** Runs `.hurl` files and the Hurl CLI's own flags,
  env vars and config file — see [docs/compat.md](docs/compat.md) and
  [docs/guides/migrate-from-hurl.md](docs/guides/migrate-from-hurl.md).
- **OpenAPI contracts.** Validate every response against a spec —
  [docs/guides/openapi.md](docs/guides/openapi.md).
- **Mock server.** `sonde mock` serves a spec's examples, or data
  generated from its schemas, before the backend exists —
  [docs/guides/mock-server.md](docs/guides/mock-server.md).
- **Streaming.** Test Server-Sent Events and WebSocket exchanges in
  `.sonde` files — [docs/guides/streaming.md](docs/guides/streaming.md).
- **gRPC.** Call unary and server-streaming gRPC methods with JSON messages,
  from `.proto` files, descriptor sets or server reflection —
  [docs/guides/grpc.md](docs/guides/grpc.md).
- **AI agents.** `sonde mcp` lets Claude Code, Cursor or VS Code list,
  check and (when you allow it, against the hosts you list) run request
  files — [docs/guides/mcp.md](docs/guides/mcp.md).
- **Data-driven runs.** Run a file once per CSV/JSON row —
  [docs/guides/data-driven.md](docs/guides/data-driven.md).
- **Environments.** Named variables and secrets in `sonde.yaml` —
  [docs/sonde-yaml.md](docs/sonde-yaml.md).
- **Reports.** JSON, JUnit, TAP and HTML —
  [docs/report-json.md](docs/report-json.md).
- **Importers.** curl, Postman, Bruno OpenCollection, `.http` files and
  OpenAPI specs — [docs/guides/import-export.md](docs/guides/import-export.md).
- **Editor support.** `sonde lsp`: VS Code extension, Neovim, any LSP
  client — [docs/guides/editors.md](docs/guides/editors.md).
- **Single static binary.** No runtime, no cgo dependencies, secure
  defaults (TLS verification on, sandboxed file access, zero telemetry) —
  [docs/architecture.md](docs/architecture.md).

## Sonde Desktop

**Sonde Desktop** is a desktop app for the same files: a text and form
editor, results with a JSON tree, environments, history, import from curl,
Postman and Bruno, and "Copy as" a curl or `sonde` command that reproduces a
run. It uses the CLI's engine, so a file that passes in the app passes in CI.
There is no account and no telemetry; history stays on your computer.

Builds for macOS (signed and notarized), Windows (not signed yet) and Linux
(AppImage) are on the releases page under `desktop/vX.Y.Z` tags; from 0.2.0
on, the app updates itself, from signed releases. A server
mode serves the app to a browser on `127.0.0.1`, for a project on a remote
machine. It is a separate program, not covered by the CLI's v1 promise. Read
[docs/desktop.md](docs/desktop.md).

## Documentation

Start at [docs/README.md](docs/README.md), or jump to
[getting started](docs/getting-started.md), the
[CLI reference](docs/cli/README.md), or
[what v1 promises to keep working](docs/stability.md).

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE) and
[TRADEMARKS.md](TRADEMARKS.md) for use of the name.
