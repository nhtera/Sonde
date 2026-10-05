# 0007: Desktop module

Status: Accepted. Date: 2026-10-01.

## Context

Sonde Desktop is a window for reading and running `.hurl` files with the
engine the CLI uses ([desktop.md](../desktop.md)). It needs a native
webview and a UI toolkit, which the CLI must not carry: the CLI is a static
binary built with `CGO_ENABLED=0` and a short dependency list
([architecture.md](../architecture.md) §1). It also needs things the
public `engine` API does not offer, and the v1 promise forbids growing that
API on a guess ([stability.md](../stability.md)).

Requirements:

- the CLI's module, build, dependencies and releases do not change;
- the app runs files exactly as the CLI does, and "Copy as" gives a command
  that reproduces a run;
- no secret reaches the page, except by an explicit reveal in the window;
- a project on a remote machine can be used from a browser, safely.

## Decision

### A nested module

The app is the Go module `github.com/nhtera/sonde/desktop`, in `desktop/`
with its own `go.mod`, `go.sum` and frontend. It imports the root module
through `replace github.com/nhtera/sonde => ../`. The root module never
imports it and gains no dependency from it: Wails, its cgo and the
frontend tree stay out of `go.mod` and out of the CLI binary.

Because the module path starts with `github.com/nhtera/sonde/`, Go lets it
import `github.com/nhtera/sonde/internal/...`. That is how it reuses the
engine's internals without a public API. `desktop/go.mod` ignores
`frontend/node_modules`, so Go files shipped in npm packages are never
built.

### Wails v3, pinned to a beta

The runtime is Wails v3, pinned to `v3.0.0-beta.26` in `desktop/go.mod`,
with `@wailsio/runtime` pinned in `frontend/package.json`. Wails v3 builds
the same Go services either as a native window or, with `-tags server`, as
an HTTP server that serves the same frontend and the same bindings. Server
mode needs that: it is the same app, not a second one.

The price is that v3 has no stable release. The API can change between
betas. So:

- the version is exact, not a range, and a bump is its own change, with the
  Playwright smoke test and the manual checklist
  ([desktop/MANUAL-TEST.md](../../desktop/MANUAL-TEST.md));
- the build tools (`wails3`, `task`) are pinned by version and module
  checksum (`desktop/scripts/tools.sha256`);
- if a beta bug blocks packaging on one OS, that OS can be packaged by hand
  while the others release;
- moving to the stable release is a separate decision and change.

### Server mode

`sonde-desktop-server` (`-tags server`, `CGO_ENABLED=0`) serves the app to a
browser on `127.0.0.1`. It is part of the product, for a project on a remote
dev box reached over an SSH tunnel, and has its own controls
([security.md](../security.md#server-mode)): a launch file holding a
one-time nonce, a session cookie plus an `X-Sonde-Token` header, `Host` and
`Origin` checks, no framing, and a smaller set of bindings (no native
dialogs, no reveal, no trash, no export, no copy with secret values).

Wails' own event socket is outside the guard and accepts a rebinding page,
so server mode does not use it: app events go through a guarded stream, and
lint allows Wails `Emit` only in `internal/emit`.

A separate test-only build, `-tags server,e2eharness`, drives the browser
tests. A check fails a release build that carries that tag or its files.

### `internal/enginex` hooks, not a public engine API

What the app needs from the engine goes through `internal/enginex`, set by
`engine`, and not through new exported symbols: the live redactor of a unit,
the request-sent and entry-skipped events, the kind of a transport error,
whether a capture was redacted, seeding cookies, a host allowlist,
interactive WebSocket dials and gRPC descriptors. A public field would
freeze a shape that one user has not proven, and every exported symbol is a
v1 promise. `make apicheck` shows no difference. A hook can be promoted
later, additively, once `go test` embedding or another front end proves its
shape (the same course as [0006](0006-mcp-server.md)).

### Shared `internal/runplan`

The CLI and the app build a run through `internal/runplan`: flags,
`HURL_*` and `SONDE_*` variables, the config file and `sonde.yaml` become
engine options and jobs. `internal/runflags` is its inverse, rendering a run
back to `sonde` arguments or a shell command. The app therefore honors the
CLI's configuration, and "Copy as › sonde" reproduces a run with the same
planning code rather than a second implementation. The same holds for test
summaries, cookie jars, data rows, request-file edits (`internal/syntaxedit`)
and the importers (`internal/convert`): moved out of `internal/cli`, shared.

### The view boundary

Nothing the engine returns reaches the page raw. `desktop/internal/view`
turns events into DTOs inside the run's own callback, with the unit's live
redactor, so a value captured in request 3 is already masked in the log of
request 3. Bodies are decoded and redacted whole, binary ones too. Cookie
values are masked everywhere. A test sweeps every event, binding result,
body URL and history file for seeded secrets.

### Versioning and release

The app has its own version, `desktop/frontend/package.json`, and is
released by tags `desktop/vX.Y.Z` with its own workflow
(`.github/workflows/release-desktop.yml`, [release.md](../release.md#desktop-release)).
`release.yml` is unchanged. `desktop/*` tags are in `.goreleaser.yaml`
`ignore_tags`, and `make tag-guards` checks that no desktop tag changes the
CLI's `git describe`. The app is outside the Go API contract: it depends on
`internal/` and may need a matching CLI change at any commit, and it is built
against the checkout, not against a published CLI tag.

## Consequences

- The CLI binary, `go.mod`, size and `go install` path are unchanged.
- The app and the CLI cannot drift in how a run is built, but a refactor of
  `internal/` must keep the desktop module compiling. CI builds and tests it
  (`make desktop-check`), and the root build does not.
- Two lock-step surfaces need care: `internal/` signatures the desktop calls
  (a break shows up as a desktop build failure, not a CLI one), and the
  enginex hooks, which are test-covered but not under apidiff.
- A pinned beta means the Wails bump is real work and a beta bug can hold up
  one OS. Server mode has no stable-API cover either.
- A second release train, with a second version number and its own signing
  secrets (Apple Developer ID; no Windows certificate yet, so the Windows
  installers are unsigned).
- Server mode is a larger attack surface than a window, and needs its own
  tests: a security suite runs against the server build.
- Updating the window app is decided in
  [0008](0008-desktop-updates.md): a signed manifest per release, checked
  against keys the app pins.
- The window app and server mode share one frontend, which must work in the
  platform webview and in a browser; the visual and behavior suites run on a
  harness build in Chromium and WebKit, and the native surfaces (dialogs,
  clipboard, quarantine, Gatekeeper) are checked by hand per release.
