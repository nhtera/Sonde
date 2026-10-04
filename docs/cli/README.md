<!-- Generated from the command tree by `make docs`. Do not edit. -->

# CLI reference

Every `sonde` command, generated from its cobra definition (`internal/cli/cli_reference.go`; `go test ./internal/cli -run TestCLIReferenceUpToDate -update`, wired into `make docs`).

- [sonde](sonde.md) — Run and test HTTP requests written in plain text
  - [sonde check](sonde_check.md) — Check request files for syntax errors
  - [sonde export](sonde_export.md) — Export request files to another format
    - [sonde export curl](sonde_export_curl.md) — Print each entry's equivalent curl command line, without sending it
  - [sonde fmt](sonde_fmt.md) — Format request files canonically
  - [sonde import](sonde_import.md) — Import request files from another format
    - [sonde import curl](sonde_import_curl.md) — Convert curl command lines to a request file, one entry per command
    - [sonde import http](sonde_import_http.md) — Convert a .http file (JetBrains HTTP Client or VS Code REST Client) to Sonde request files
    - [sonde import openapi](sonde_import_openapi.md) — Generate request files from an OpenAPI spec, one per operation
    - [sonde import opencollection](sonde_import_opencollection.md) — Import a Bruno OpenCollection YAML collection (a file, a directory or a zip)
    - [sonde import postman](sonde_import_postman.md) — Convert a Postman v2.1 collection to request files
  - [sonde lsp](sonde_lsp.md) — Run the language server over stdio
  - [sonde mcp](sonde_mcp.md) — Serve request files to AI agents over MCP
  - [sonde mock](sonde_mock.md) — Serve a mock of an OpenAPI spec
  - [sonde run](sonde_run.md) — Run request files (same as `sonde FILE...`)
  - [sonde test](sonde_test.md) — Run request files in test mode (same as `sonde --test FILE...`)
  - [sonde version](sonde_version.md) — Print version, commit, build date and Go version
