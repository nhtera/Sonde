# Sonde Desktop

The desktop app reads and runs `.hurl` files with the same engine as the
`sonde` CLI. It is a nested Go module (`github.com/nhtera/sonde/desktop`)
that imports the root module through `replace github.com/nhtera/sonde => ../`;
the root module never imports it and gains no dependency from it.

- **Runtime:** [Wails](https://v3.wails.io) pinned to **v3.0.0-beta.26** (`go.mod`,
  `@wailsio/runtime` in `frontend/package.json`). A bump is its own change,
  with the smoke test.
- **Frontend:** React 19, TypeScript, Vite; ESLint, Vitest, Playwright.
- **Tools:** `wails3` and `task`, pinned by version and module checksum in
  [`scripts/tools.sha256`](scripts/tools.sha256). Install and verify them into `../bin`
  with `make desktop-tools` from the repository root.

## Builds

| Build | Command | Notes |
|---|---|---|
| Desktop window | `../bin/task build` | cgo and the platform webview |
| Server mode | `../bin/task build:server` | `-tags server`, `CGO_ENABLED=0`, shipped |
| E2E harness | `../bin/task build:harness` | `-tags "server e2eharness"`, test-only, never released |
| Native spike | `go build -tags e2eharness` (after `npm run build:harness`) | window build with the harness bindings, for measuring the spikes in the platform webview; test-only, never released |

The frontend's TypeScript bindings are generated, not committed: `make
desktop-bindings` writes them to `frontend/bindings/` (the check and e2e
targets run it first). `make desktop-check` (repository root) runs `go mod tidy -diff` and the
license allowlist, then vet, tests and lint for the server (plain and
`production`) and harness builds. It also installs the frontend
dependencies with `npm ci --ignore-scripts`, checks the install-script
allowlist, and runs lint, typecheck and unit tests. `make desktop-e2e`
builds server mode, the harness and the fixture API, then runs the
Playwright tests: server mode's own sign-in, the shell on a copy of
`testdata/shop-api`, and the tree on 1000 generated files (install the browsers with
`npx playwright install chromium webkit` in `frontend/`). `make
lint-desktop-native` lints the window build on a machine with cgo and the
webview; `make desktop-vuln` scans the module. `desktop/go.mod` ignores
`frontend/node_modules`, so Go files shipped in npm packages are never built. `make
desktop-record` re-records the shop-api run events that the run component
tests replay.

The services live in `internal/host`: `host.go` builds what they share
(the `Host`) and keeps the registry, and every other file there registers
one service from `init`, for the modes that get it. Adding a service adds
a file; `host.go` does not change. The root package holds only the entry
points (`main.go`, `main_server.go`, `main_harness.go`, `serve.go`).

## Server mode

`sonde-desktop-server --root <project> [--port N] [--open] [--data DIR]` serves the app to a
browser on `127.0.0.1` only, for example on a remote machine reached through
`ssh -L N:127.0.0.1:N` (forward the same port number). It prints the address
without any secret. It then writes a launch page, private to the user
(0600), into the user cache directory (`Sonde/launch-<port>.html`). The page
holds a one-time link, valid for 60 seconds, and is written once the port is
listening. `--open` opens that page in the default browser. Press Enter for a
fresh link, which revokes the previous one. The file is removed when its
link is used, expires, or the server stops. `--data` puts the app data in
`DIR/config` and `DIR/cache` instead of the user folders. A sandboxed
browser that cannot read the cache folder (for example snap Firefox on
Ubuntu) needs the link copied from the file instead.

Each request passes [`internal/serverauth`](internal/serverauth):

- Host must be `127.0.0.1:<port>` or `localhost:<port>`.
- Origin, when sent, must be this server. Requests other than GET and HEAD
  must send it. A cross-site or same-site `Sec-Fetch-Site` is refused.
- The link's nonce is exchanged once for an `HttpOnly`, `SameSite=Strict`
  cookie. The page then reads the per-launch token and sends it as
  `X-Sonde-Token` on every runtime call.
- Assets and body URLs need the cookie; runtime calls need both.
- Framing is denied everywhere (`frame-ancestors 'none'`,
  `X-Frame-Options: DENY`), and no response carries CORS headers.

Wails serves `/health`, `/wails/custom.js`, and its event and stream
WebSockets outside this guard. Its event socket accepts a DNS-rebinding
page, because the library allows an Origin that matches the Host. It also
accepts any local process that sends no Origin. So:

- server mode registers no stream handlers;
- app events go only through the guarded endpoints (the server variant of
  `internal/emit`), never through Wails' event socket;
- lint forbids Wails `Emit` outside `internal/emit` and the harness;
- a server-mode test listens on the event socket as a rebinding page and
  fails on any frame.

Responses may take up to 12 hours to write (binding calls answer when their
work is done, and bodies stream through slow tunnels); each request still
ends with its client.

## Spikes

Numbers from 2026-09-30 on an Apple M1 (macOS 15.7) unless noted.

### 1. Language server per frontend session: go (Go side); client transport open

`internal/lspbridge` runs `lsp.NewServer` + `Run` over `io.Pipe` with
Content-Length framing; the client side sends and receives bare JSON, which is
exactly the `Transport` of `@codemirror/lsp-client` 6.3 (`send`, `subscribe`,
`unsubscribe`).

- A reload opens a new session, whose `initialize` succeeds. A second
  `initialize` on one session is refused, as the server requires.
- The session ends and its pipes close when its context is cancelled, on
  `Close`, and on `shutdown` plus `exit`.
- didOpen to publishDiagnostics, in process: **0.7 ms** worst of 5 (budget 150 ms,
  so the budget is left to the binding call and event hop).
- Wails cancels a window's pending calls only when the window is destroyed,
  not on reload. The bridge (Phase 4) therefore keys sessions by window and
  closes a window's older sessions when its runtime reports ready again.
  It also closes a session, best effort, on `pagehide`.

- `Close` breaks both pipes first, so a stalled consumer or a blocked
  `Send` cannot hold it up.

Chosen: the bridge. Client messages go through an `LSP.Send(session, msg)`
binding. Server messages go through `internal/emit`, carrying the session
id and addressed to that session only, not through a broadcast Wails
event. The fallback (exported helpers) is not needed.

Still open: the CodeMirror `Transport` over the binding and the emitter is
built with the service (Phase 4) and the editor (Phase 7). The 150 ms
budget is re-measured end to end there.

### 2. Wails v3 surface: go

- **Services:** `application.NewService` / `NewServiceWithOptions(&T{},
  ServiceOptions{Name, Route})`; `ServiceStartup(ctx, options)`,
  `ServiceShutdown()`. A service implementing `http.Handler` is mounted at
  `Route` on the asset server. Its handler sees the path with `Route`
  removed.
- **Bindings:** `wails3 generate bindings -f '<build flags>' -ts -i`. Services
  registered through `internal/host`'s registry are found; `Call.ByName` (the
  type's package-qualified name) and the generated `Call.ByID` both work.
  Runtime calls are `POST /wails/runtime` with a JSON body, sent through
  `window.fetch` (server mode adds the token there).
- **Events:** `application.RegisterEvent[T](name)`, `app.Event.Emit(name, data)`;
  JS `Events.On(name, cb)`. An emitted event reaches every window, and in
  server mode every client of the unguarded event socket. Only
  `internal/emit` sends events.
- **Cancellation:** `CancellablePromise.cancel()` in JS cancels the bound
  method's `context.Context`, observed in Go **23 ms** after the cancel
  (Chromium, WebKit, and the native macOS window).
- **Dialogs:** `app.Dialog.OpenFile()/OpenFileWithOptions/SaveFile()/Info()/Question()/Warning()/Error()`.
- **Menus:** `app.Menu`. **Title bar:** `MacWindow{TitleBar: MacTitleBarHiddenInset,
  InvisibleTitleBarHeight}`. **Drag regions:** CSS `--wails-draggable: drag`.
- **Asset middleware:** `AssetOptions.Middleware` wraps everything served by the
  asset server, runtime calls included (server mode's guard).
- **Clipboard:** only `SetText`/`Text`. There is no concealed or transient
  type, so Phase 4's `clipboard` adds a small platform shim for secret
  copies:
  - macOS: `org.nspasteboard.ConcealedType` and `TransientType`;
  - Windows: `ExcludeClipboardContentFromMonitorProcessing`;
  - all platforms: the text is cleared after 60 s if it is unchanged.

### 3. Bodies over a URL: go (macOS, Chromium, WebKit); WebKitGTK open

A Go store serves `/_sonde/body/<id>` in 64 KiB chunks and checks the request
context before each chunk.

| Runtime | 50 MB fetch | Worker `JSON.parse` (560k items) | Cancel mid-stream |
|---|---|---|---|
| macOS window (WKWebView, custom scheme) | 34 ms | 106 ms | handler stopped after 1.0 MB of 50 MB |
| Chromium (harness) | 35 ms | 124 ms | stopped after 1.0 MB |
| WebKit (Playwright, harness) | 27 ms | 183 ms | stopped after 1.4 MB |

- Cancellation reaches the handler through `AbortController`: the request
  context ends and the handler stops.
- Over loopback an unthrottled 50 MB body is fully sent before the abort
  lands. The measurement therefore uses a body served at about 12 MB/s.
  No 2 MB range windows are needed.
- `Content-Security-Policy: sandbox` is honored by the custom scheme. In the
  macOS window, a control page without the policy runs its script in an
  iframe, and the sandboxed page does not.
- In the browsers, a popup of the control page runs its script and the
  sandboxed one does not. Every iframe is refused by the guard's
  `frame-ancestors 'none'`.
- WebKitGTK (Linux) has not been measured yet. It runs in the Linux release
  job, where the budget from the risk table applies: over 5 s means raw
  view limits above 10 MB.

### 4. Playwright against the harness: go

`frontend/e2e/smoke.spec.ts` loads the app, calls a binding and receives an event
(Chromium and WebKit, about 2 s), and checks that a call without the token is
refused. `frontend/e2e/server-mode.spec.ts` runs the server build's sign-in:

- the launch page opens as a file;
- its cross-site navigation trades the nonce for the SameSite=Strict
  cookie;
- a runtime call with the page's token succeeds, and the same call without
  it gets 401;
- a browser without the link gets 401. `E2E_PORT` picks the port (default 34115); `E2E_BASE_URL` targets an
already running harness. `E2E_SPIKES=1` also runs the spike measurements.
