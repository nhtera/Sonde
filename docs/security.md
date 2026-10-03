# Security

This page explains what Sonde treats as trusted, what it does to limit the
damage untrusted input can do, and how to report a vulnerability. It is the
user-facing companion to [architecture.md](architecture.md) §9 (Security
Model), which is the developer-facing version of the same table.

To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Trust model

- **Trusted:** the command line you type, flags, environment variables you
  set yourself, and files under your user config directory.
- **Untrusted:** a request file (`.hurl`) and a `sonde.yaml` you did not
  write yourself — for example one checked out from a repository someone
  else controls, or a collection someone sent you. Sonde is designed so
  that running an untrusted request file or `sonde.yaml` cannot read or
  write files outside the project, cannot change what files a run is
  allowed to touch, and cannot leak a secret you supplied on the trusted
  command line.

## Secrets never appear in the clear

Every secret Sonde ever holds — a `--secret` value, a `SONDE_SECRET_*`
environment variable, a `--data-secret` data-file column, a `sonde.yaml`
`secrets_files` entry, or a value captured at runtime with `redact` — is
masked out of everything Sonde writes: the terminal (including `-v` and
`-vv`), `--json`, every report format (`--report-junit`, `--report-tap`,
`--report-json`, `--report-html`), the `--curl` command file, `sonde export
curl`, and the `--cookie-jar` file. Masking also catches the value's
base64, URL-escaped and JSON-escaped forms, so a secret that a server
happens to echo back re-encoded still comes out as `***`.

A short secret (under 4 characters) is still masked, but Sonde warns that a
short value is prone to matching text it was never meant to redact — pick a
longer one where you can.

## Local files are sandboxed

An option that names a file *inside a request file* — `file,` request
bodies and multipart parts, `output`, `--cacert`/`--cert`/`--key`,
`--netrc-file`, `--unix-socket` — is confined to a file root (the current
directory, or wherever `--file-root` points): `..`, an absolute path, or a
symbolic link that would leave the root are all rejected. The same option
given directly on the command line is trusted and not sandboxed, since you
typed it yourself.

`sonde.yaml` is untrusted input with its own, stricter rule: a
`variables_files` or `secrets_files` entry must be a relative path that
stays inside the directory holding that `sonde.yaml` — even an absolute
path is rejected outright, not just one that escapes via `..` or a
symlink. A `sonde.yaml` can never widen what a run is allowed to touch; it
can only be more specific about variables and secrets within its own
directory. Sonde also refuses to use a candidate `sonde.yaml` that is
writable by anyone other than its owner.

## No ambient credentials on the wrong host

A `~/.netrc` credential is only ever sent to the exact host, port and
scheme it was written for. When a redirect or `--resolve`/`--connect-to`
sends a request somewhere else, that credential does not follow it.

## Templates cannot read the environment

There is no `getEnv` function in a request file's templates (Hurl 8 does
not have one either). A future built-in that needs process environment
access will be opt-in, allowlisted, and treated as a secret automatically.

## Reports and imports

The HTML report (`--report-html`) renders every value — URLs, headers,
bodies, filenames, assert messages, captures — through Go's `html/template`
autoescaper, sends a strict Content-Security-Policy
(`default-src 'none'; style-src 'unsafe-inline'`), and loads nothing from
the network: no remote fonts, scripts, or images.

Every importer (`sonde import curl|postman|opencollection|http`, `sonde
import openapi`) is exercised by a fuzz target, runs no code found in its
input (an imported script becomes a comment, never something Sonde
executes), and caps output size. `--openapi` never fetches a remote `$ref`
unless you pass `--openapi-allow-remote`.

A decoded response body is capped (512 MiB by default) so a decompression
bomb cannot exhaust memory.

## The LSP (`sonde lsp`)

The language server treats `sonde.yaml`, `variables_files` and
`secrets_files` exactly like a run does: as untrusted input, read only to
extract variable *names* for completion and hover. A secrets_files value
is parsed only far enough to detect a name clash; the value itself is
never kept, never shown in a hover, and never sent anywhere. The LSP never
makes an HTTP request and never reads a file outside the workspace folders
the client gave it.

## AI agents (`sonde mcp`)

`sonde mcp` serves request files to an AI agent, which Sonde treats as
untrusted: it may be following instructions planted in a file, a web page
or a response body. The server starts read-only, and everything else is
granted on its command line, never by a tool call:

- **Running files** needs `--allow-run` plus at least one `--allow-host`.
  Without them the run tool does not exist.
- **Hosts** are checked twice with the same matcher: the URL of every
  request, redirect and WebSocket handshake, and every connection Sonde
  opens, proxies included. Entries using `proxy`, `connect-to`, `resolve`,
  `unix-socket`, the `netrc` options or `output` fail without sending
  anything.
- **Files**: tools read only `.hurl` and `.sonde` files under `--root`.
  Runs are confined to that root, and a `sonde.yaml` above it is ignored.
- **Secrets** come only from `sonde.yaml`, `SONDE_SECRET_*` and the
  server's own flags, and are redacted from every tool result and from the
  audit log it writes to standard error.

The allowlist is a statement of trust in host names: a secret a request
file sends to an allowed host goes there, as in a manual run. Response
bodies returned to the agent are data from the server under test. See
[guides/mcp.md](guides/mcp.md) and
[decisions/0006-mcp-server.md](decisions/0006-mcp-server.md).

## Sonde Desktop

[Sonde Desktop](desktop.md) runs the same engine, so everything above holds
for its runs. It adds a page (a webview) that must never receive a secret,
native actions on your files and clipboard, and a local HTTP server mode.
The trust model is the CLI's: the project folder's request files and
`sonde.yaml` are untrusted input; what you do in the window is trusted. The
page itself is treated as less trusted than the Go side: it receives only
values that have been redacted there, and it never names a path of its own.

| Threat | Control |
|---|---|
| A response body runs script in the app (HTML preview, SVG, PDF) | bodies are served from `/_sonde/body/<id>` with `Content-Security-Policy: sandbox; default-src 'none'`, `X-Content-Type-Options: nosniff` and `no-store`; the preview frame has an empty `sandbox` attribute (no scripts, no top navigation) and no referrer; the app's own CSP allows scripts from itself only and no remote assets (the script that sets the theme before the first paint is a file, `theme-boot.js`, not an inline script; the page's `<html>` carries the theme settings as attributes, which are theme ids from an allowlist, escaped; in server mode they are the only settings the page shows before sign-in) |
| The page reads or writes files it should not | the page never sends a path to a native action: a native dialog returns a **handle** that is valid once, for 5 minutes, and only for the kind of dialog that made it (open file, open folder, save); project paths resolve through `internal/sandbox` (`os.Root`) and stay in the project folder; dot paths, `*.secrets` files and the project's `secrets_files` are refused to read, write, rename and use as bodies; secrets are edited only through the Environments service; writes refuse dot names at any level |
| A secret reaches the page (events, logs, results, errors, body URLs, history) | **the view redaction boundary** (`desktop/internal/view`): every event is converted to a DTO inside the run's own callback with that unit's live redactor, which learns each secret as it is captured. An attempt that captures a secret holds its own log lines until it finishes, so a value printed before it became secret is still masked; data-row secrets are live per row. Response bodies are decoded first and then redacted whole, **binary bodies included**; a body that does not decode is not stored. Every cookie value, `[Cookies]` line and curl `--cookie` is masked. A sentinel suite seeds secrets (declared, data-row, `redact` captures, binary and gzip bodies, session cookies) and sweeps every binding result, event, body URL and history file for them |
| Run history leaks credentials at rest | history is stored in the app's config folder (0600), without bodies and without curl commands; it masks the run's secrets, credential header values, **every cookie value**, and captured tokens by name (token, secret, key, password, session, auth, cookie, jwt, bearer) or JWT shape; retention 7 days, 30 days (default) or forever, and it can be turned off. The name heuristic can miss a secret under an ordinary name: declare it secret |
| A repository's git config runs a program (filter driver, hook, fsmonitor) | the folder is untrusted until you trust it: the branch is read from `.git/HEAD` without running git; status and commit need the trust (kept in `trust.json`); every git run disables fsmonitor and hooks, ignores the system config, never prompts and times out at 30 seconds; a commit in a trusted folder runs your hooks; secrets files are flagged in the Changes card and refused by Commit |
| "Open in default app" runs hostile content | the file holds the **redacted** body, in a private (0700) temp folder, with an extension from an allowlist (`.pdf`, `.png`, `.jpg`, `.gif`, `.txt`, `.json`, `.xml`; anything else, HTML included, opens as `.txt`), marked as downloaded (quarantine on macOS, Mark of the Web on Windows); programs are started with argument lists, never a shell; the files are removed when the app quits. The window app only |
| A revealed secret lingers on the clipboard or crosses the page | reveal needs an explicit action (⌥ and a confirmation naming where the text goes); the text is written from Go and never passes through the page; it is marked concealed or transient where the system supports it (macOS: `org.nspasteboard.ConcealedType`; Windows: excluded from clipboard history) and cleared after 60 seconds if the clipboard still holds it. Without reveal, copied commands carry variable references or masked values. The window app only |
| Import writes outside the project, runs scripts or leaks a credential | the converters are the CLI's: no script is executed (it becomes a comment), input is capped at 64 MiB, output is confined to the project folder by `internal/sandbox`; a pasted command's credentials (`Authorization`, `-u`, API key headers and query values) are lifted into the environment's secrets file (0600) and the preview shows references, redacted; a pasted or uploaded input is staged once in the app's cache, which is emptied at start and when a project opens |
| A Postman suggestion injects an assert or request | suggestions are derived by text patterns from parsed script events, never from comments, and nothing is executed; a suggestion is shown as a diff and applies only when you accept it; the result is reparsed to check that exactly one row was added, and refused if the file changed since; accepting twice cannot apply it twice |
| Environment edits corrupt `sonde.yaml` or lose a secret | an edit is validated against every environment, written atomically, and journaled with the hash of each write, so an interrupted edit is rolled back at the next start only for files still holding the edit's bytes; a secret's value is written to its secrets file, never to `sonde.yaml` |

### Server mode

`sonde-desktop-server` lets a browser drive a request runner that can read
the project and reach the network, so anything that can reach its port is a
threat: another local user or process, a web page in the same browser, a page
that rebinds a DNS name to 127.0.0.1.

| Threat | Control |
|---|---|
| Another machine reaches the server | it listens on `127.0.0.1` only; there is no flag to change that (`WAILS_SERVER_HOST` and `WAILS_SERVER_PORT` are cleared at start). Remote use goes through an SSH tunnel |
| A local process or another browser tab calls it | every runtime call needs a per-launch **token** in the `X-Sonde-Token` header, which a browser can send only from the app's own page (a cross-site page cannot add the header to a request without a preflight the server never answers, and no response carries CORS headers); the page gets the token from an endpoint that needs the session cookie. Assets and body URLs need the cookie |
| The launch link leaks (process list, shell history, logs) | the link is **not** on a command line or in a log: it is in a launch file (0600, user cache folder) holding a one-time nonce, valid for 60 seconds, revoked by a newer link, removed on use, on expiry and at exit, and minted only after the port is confirmed to be this server; the nonce is traded once for an `HttpOnly`, `SameSite=Strict` cookie; request logging is off so a link is never recorded. Nonces, cookies and tokens are compared in constant time |
| DNS rebinding, or a page on another port (cookies are shared across ports of 127.0.0.1) | `Host` must be `127.0.0.1:<port>` or `localhost:<port>`; `Origin`, when sent, must be this server, and every request other than GET and HEAD must send one; a `Sec-Fetch-Site` of `same-site` or `cross-site` is refused |
| The app is framed (clickjacking) | `Content-Security-Policy: frame-ancestors 'none'` and `X-Frame-Options: DENY` on every response. The one exception is a response body, which only the app's own page may frame, sandboxed by the body's own policy |
| Wails' event socket (outside the guard, accepts a rebinding page) | not used: server mode registers no stream handlers, app events go through a guarded stream (`/_sonde/events`, token required), lint allows Wails `Emit` only in `internal/emit`, and a test connects as a rebinding page and fails on any frame |
| The page does something only a local user should (open or reveal files, trash, export, copy secrets) | **reduced bindings**: server mode binds no native dialog, reveal, trash, report export, save response, open externally or reveal-copy, and the window-only services are absent, which a test checks per mode. Imports use an upload (64 MiB) instead of a path on the server |
| Secrets cross to the browser | the same view boundary as the window; there is no reveal in server mode: Copy as always masks, and nothing writes to a clipboard from Go |
| A slow or stuck client holds resources | reads time out after 1 minute; a lagging event stream client is dropped (32 MiB cap) and reconnects and resynchronizes |

## Supply chain

Dependencies are kept minimal and reviewed with `go mod why`; `govulncheck`
and `gosec` (via `golangci-lint`) run in CI on every push, plus a nightly
fuzz run (`.github/workflows/nightly-fuzz.yml`) across every parser and
importer. CI workflows request only `contents: read`, and third-party
GitHub Actions are pinned by commit SHA. Releases are signed (`cosign`,
keyless via GitHub OIDC) and ship a Software Bill of Materials (`syft`);
publish credentials live only in the protected `release` environment.
The desktop app's releases add build provenance attestations that the
signing jobs verify before they sign anything
([release.md](release.md#desktop-release)). One known gap: the AppImage's
`linuxdeploy` is pinned and checked, but the `AppRun` that Wails downloads
from the archived AppImageKit "continuous" release is not, nor is the
AppImage runtime (the first code an AppImage runs) that linuxdeploy's
AppImage plugin may download while packaging.

## Reporting a vulnerability

See [SECURITY.md](../SECURITY.md): please report privately through GitHub
Security Advisories rather than a public issue.
