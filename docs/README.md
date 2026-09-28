# Sonde Documentation

| Document | Contents |
|---|---|
| [getting-started.md](getting-started.md) | Install, first file, run, asserts/captures, test mode & reports, environments |
| [file-format.md](file-format.md) | What a Sonde file is, `.hurl` vs `.sonde`, links to the shared Hurl grammar |
| [architecture.md](architecture.md) | Package map, public API, exit codes, variable precedence, redaction, file access, HTTP semantics, libraries, testing, security |
| [compat.md](compat.md) | Hurl 8.0.1 compatibility: queries, filters, predicates, functions, options, CLI flags, env vars, config, differences (generated, `make docs`) |
| [sonde-yaml.md](sonde-yaml.md) | Project file: environments, variables and secrets files, defaults |
| [report-json.md](report-json.md) | JSON result schema shared by `--json` and `--report-json` |
| [conformance.md](conformance.md) | Conformance harness, lanes, manifest and gate |
| [benchmarks.md](benchmarks.md) | Binary size, startup, parse, engine and parallel-run baselines |
| [stability.md](stability.md) | The v1 promise: compat level, CLI/flags, exit codes, JSON schema, `sonde.yaml`, Go API |
| [security.md](security.md) | Trust model: request files and `sonde.yaml` are untrusted input, the CLI is trusted |
| [release.md](release.md) | Release runbook: prerequisites, cutting an rc/final, verifying signatures, roll-forward |
| [cli/README.md](cli/README.md) | Every command and flag, one page each (generated, `make docs`) |
| [guides/go-test.md](guides/go-test.md) | Running request files from `go test` |
| [guides/ci-github-actions.md](guides/ci-github-actions.md) | Running tests and keeping reports in GitHub Actions |
| [guides/data-driven.md](guides/data-driven.md) | Running files once per row of a CSV or JSON data file (`--data`) |
| [guides/openapi.md](guides/openapi.md) | OpenAPI contracts: `--openapi` validation, `sonde.yaml` `openapi:`, `sonde import openapi` |
| [guides/mock-server.md](guides/mock-server.md) | `sonde mock`: serve an OpenAPI spec's examples and generated responses |
| [guides/streaming.md](guides/streaming.md) | Server-Sent Events and WebSocket tests in `.sonde` files |
| [guides/grpc.md](guides/grpc.md) | gRPC calls in `.sonde` files |
| [guides/import-export.md](guides/import-export.md) | `sonde import` and `sonde export curl`: common flags, writer rules, kinds |
| [guides/migrate-from-hurl.md](guides/migrate-from-hurl.md) | Switching from Hurl: commands, flags, differences, what Sonde adds |
| [guides/migrate-from-postman.md](guides/migrate-from-postman.md) | Importing Postman collections and environments |
| [guides/editors.md](guides/editors.md) | `sonde lsp`: VS Code extension, Neovim, and any other LSP client |

Contributor guide: [CONTRIBUTING.md](../CONTRIBUTING.md). Security reports: [SECURITY.md](../SECURITY.md).
