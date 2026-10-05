# Contributing to Sonde

Thanks for helping! Small, focused pull requests are easiest to review.

## Developer Certificate of Origin

Every commit must be signed off under the
[Developer Certificate of Origin](https://developercertificate.org/):

```sh
git commit -s -m "feat(cli): add version command"
```

The sign-off (`Signed-off-by: Your Name <you@example.com>`) certifies that you
wrote the change or have the right to submit it under Apache-2.0. A DCO check
runs on every pull request.

## Development setup

- Go 1.26 or newer (the repository pins toolchain `go1.27.1` in `go.mod`;
  with the default `GOTOOLCHAIN=auto` Go downloads it for you)
- `make`, `bash`, `git`
- Optional: [GoReleaser](https://goreleaser.com) v2 for `make snapshot`

Linters and scanners are pinned in the `Makefile` and installed into `./bin`
on first use (`make tools`).

| Target | What it does |
|---|---|
| `make build` | static `bin/sonde` (`CGO_ENABLED=0`, `-trimpath`) |
| `make test` | unit tests |
| `make race` | unit tests with the race detector |
| `make lint` | golangci-lint (includes package-boundary rules) |
| `make vuln` | govulncheck |
| `make license-check` | SPDX headers + dependency license allowlist |
| `make fuzz-smoke` | every fuzz target for `FUZZTIME` (default 10s) |
| `make snapshot` | local GoReleaser build into `dist/` |
| `make conformance` | Hurl conformance suite (not available yet) |
| `make docs` | generated docs (not available yet) |

Before opening a pull request run `make build test lint license-check`.

### Website

The landing page and docs at https://sonde.erai.dev live in `site/` (Node
22.18+): `make site-dev` serves it, `make site-check` runs its checks and
build. `docs/` is the source of the docs pages; edit the Markdown there,
never a copy under `site/`. See [site/README.md](site/README.md).

## Conventions

- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `ci:`).
- **File names:** Go sources `snake_case.go`; shell scripts `kebab-case.sh`.
- **License header:** every `.go` file starts with

  ```go
  // Copyright 2026 The Sonde Authors
  // SPDX-License-Identifier: Apache-2.0
  ```

- **Package boundaries:** allowed imports between packages are defined in
  [docs/architecture.md](docs/architecture.md) §2 and enforced by `depguard`.
- **Public contracts:** a change that adds or changes an exported symbol, CLI
  flag, exit code, config key or report field also updates
  `docs/architecture.md` in the same pull request.
- **Dependencies:** only Apache-2.0, MIT, BSD-2-Clause, BSD-3-Clause or ISC
  licenses; no cgo.
