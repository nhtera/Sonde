# Sonde Desktop

Sonde Desktop reads and runs `.hurl` and `.sonde` files in a window, with the
same engine as the `sonde` CLI. A file that passes in the app passes in CI
and for [AI agents](guides/mcp.md): the app builds its runs with the code
the CLI uses (`internal/runplan`), so the same flags, environment variables,
config file and `sonde.yaml` apply.

There is no account and no telemetry. The app sends the requests you wrote
and nothing else. Run history stays on your computer.

The desktop app is a separate program with its own version. It is not part of
the CLI's v1 contract: see [stability.md](stability.md#sonde-desktop). The design
is recorded in [decisions/0007-desktop-module.md](decisions/0007-desktop-module.md),
the trust model in [security.md](security.md#sonde-desktop).

## Install

Releases are on the [releases page](https://github.com/nhtera/sonde/releases)
under tags named `desktop/vX.Y.Z`, apart from the CLI's `vX.Y.Z` releases.
Each carries:

| File | For |
|---|---|
| `Sonde-Desktop-X.Y.Z-macos-universal.dmg` | macOS 12 or later, Apple silicon and Intel |
| `Sonde-Desktop-X.Y.Z-windows-amd64-setup.exe` | Windows, x64 |
| `Sonde-Desktop-X.Y.Z-windows-arm64-setup.exe` | Windows, ARM64 |
| `Sonde-Desktop-X.Y.Z-linux-x86_64.AppImage` | Linux, x86_64 |
| `Sonde-Desktop-Server-X.Y.Z-OS-ARCH[.exe]` | server mode (below), for linux, darwin and windows on amd64 and arm64 |
| `Sonde-Desktop-X.Y.Z.sbom.spdx.json` | the software bill of materials |
| `checksums.txt`, `checksums.txt.sigstore.json` | SHA-256 of every file above, and its cosign signature |

### macOS

The disk image is signed with a Developer ID certificate and notarized by
Apple, so Gatekeeper opens it without a warning. Open the image and copy the
app to Applications.

### Windows

The installers are **not signed yet**. On first launch Windows SmartScreen
shows "Windows protected your PC": choose **More info**, then **Run anyway**.
Because nothing vouches for the file but this project, verify the download
against `checksums.txt` first (see [Verifying downloads](#verifying-downloads)).

The installer also installs the WebView2 runtime if the computer has none.

### Linux

The AppImage is built on Ubuntu 24.04 against GTK 4 and WebKitGTK 6.0. The
supported systems are Ubuntu 24.04 or later, Debian 13 and Fedora 40 or
later. Only x86_64 is built, and there is no GTK 3 build.

```sh
chmod +x Sonde-Desktop-X.Y.Z-linux-x86_64.AppImage
./Sonde-Desktop-X.Y.Z-linux-x86_64.AppImage
```

If it does not start, check that the system has GTK 4 and WebKitGTK 6.0
(Debian and Ubuntu: `libgtk-4-1` and `libwebkitgtk-6.0-4`; Fedora: `gtk4`
and `webkitgtk6.0`).

### Verifying downloads

`checksums.txt` is signed by the release workflow (cosign, keyless, GitHub
OIDC). Verify it, then check your file against it:

```sh
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/nhtera/Sonde/\.github/workflows/release-desktop\.yml@refs/tags/desktop/v[0-9].*$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum -c --ignore-missing checksums.txt     # shasum -a 256 -c on macOS
```

The repository name is case-sensitive in the identity: `nhtera/Sonde`.

Every downloadable file (the disk image, the installers, the AppImage and
the server binaries; not `checksums.txt`, the SBOM or the bundle) also has a
build provenance attestation made by the release workflow, that you can
check with the GitHub CLI:

```sh
gh attestation verify Sonde-Desktop-X.Y.Z-linux-x86_64.AppImage \
  --repo nhtera/Sonde \
  --signer-workflow nhtera/Sonde/.github/workflows/release-desktop.yml
```

For the disk image you can also run `spctl -a -vv -t install FILE.dmg` and
`xcrun stapler validate FILE.dmg`.
[release.md](release.md#desktop-release) says what the workflow verifies
before it signs anything.

## First run

The app opens on a welcome screen. **Open a folder** (⌘O, Ctrl+O) picks a
project folder; recent folders are remembered. A project is a
folder of request files, usually with a `sonde.yaml` in it
([sonde-yaml.md](sonde-yaml.md)).

You can also start it with a folder: `sonde-desktop --root DIR`. On macOS
the program is `Sonde.app/Contents/MacOS/sonde-desktop` inside the app.

The side panels are Files, History, Test run, Environments, Contract & mock,
AI agents and Settings. The editor has two views of a file, Text and Form
(⌘E switches). The Form edits the same text: every change is one splice into
the file, checked by parsing the result, so comments and layout stay. A row
the Form cannot write safely (a multi-line value, a tagged body) is
read-only there and says "Edit in Text".

### The file root is the project folder

The folder you open is the file root. A `file,` body, a multipart part, an
`output` path or a certificate named in a request file resolves inside that
folder and nowhere else: `..`, absolute paths and links that leave it are
refused, as with `--file-root .` on the CLI
([architecture.md](architecture.md#file-access)). Opening a different folder
resets the open tabs, runs, Send sessions, session overrides and the mock.

The app never reads or writes a path starting with `.` inside the project,
`*.secrets` files, or the files `sonde.yaml` lists as `secrets_files`: secrets
are edited in the Environments panel, and the Files tree shows the secrets
files but cannot open them.

### Unsaved edits

A run uses the text in the tab, saved or not. Closing a tab, opening another
folder or leaving the page with unsaved edits asks first. If a file changes
on disk while you have unsaved edits, the tab offers Reload or Keep mine;
the app does not merge silently. The app's own saves do not trigger that
prompt.

## Running and Send

| Action | Keys | What it runs |
|---|---|---|
| Run file | ⌘R | every request in order |
| Run to cursor | ⇧⌘↵ | requests 1 to N, N being the request at the cursor |
| Send | ⌘↵ | request N alone, reusing the earlier requests' results |
| Test run | Test run panel | the chosen files in test mode, like `sonde --test`; Parallel jobs is `--jobs`, "Continue after a failed request" is `--continue-on-error` |
| Data run | **Data** in the editor's toolbar, or "Run with a data file…" | the file once per row of a CSV or JSON file (`--data`); the results show a pill per row |

A project data file's columns named like credentials (`password`, `pass`,
`token`, `secret`…) are treated as `--data-secret` columns: their values are
masked in everything the app shows. Send and Run to cursor are not available
while a data file is picked: they would miss the row's variables.

### Send semantics

Send runs one request without repeating the ones before it. It can do that
only when it knows what they produced, so a full run of the file leaves a
**session**: the captures of every request and the cookie jar.

- Send N reuses the session only if requests 1 to N-1, the environment, the
  overrides (values included), the resolved project values and the data row
  are the same as in that run. Otherwise it refuses with "Results are from
  another version or environment · Run 1–N" and nothing is sent.
- With no session yet, Send N runs requests 1 to N, and that run becomes the
  session.
- A Send adds its own captures to the session, so a second Send sees them.
  Where two requests capture the same name, the last one wins.
- **Cookies are always seeded from the session's jar**, whether or not
  "Keep cookies" is on. Send 5 after a login in request 1 sends the session
  cookie.
- Only a run that was not stopped is stored as a session. One run per file
  at a time.
- Edited text above request N makes the session stale; the results panel
  shows a banner, and the earlier run's results stay, dimmed and marked
  "from the run at hh:mm".

**Keep cookies** (Settings › Cookies) is only persistence. With it on, the
cookie jar at the end of a full run of one file is stored, and the next full
run of that file starts from it, as if you had passed `-b FILE`. With it off,
every full run starts with an empty jar. The kept jar holds real cookie
values: it is a file in the app's config folder, mode 0600, and you can list
and delete the cookies from the cookie jar view (names, paths, expiry and
flags are shown; values never are).

## Environments and overrides

The Environments panel lists the `sonde.yaml` environments, their variables
and secrets. **Mark secret** moves a variable's value out of `sonde.yaml`
into the environment's secrets file (mode 0600; `secrets/<env>.secrets` is
created and added to `secrets_files` if it has none). Edits go to the file that defines the value, are validated against
every environment, and a journal in the app's config folder rolls back an
unfinished edit at the next start.

A **session override** sets a variable for one session (a `--variable`):
it is gone when you open another folder or quit. A value overridden under a
secret's name is shown as `***` after you enter it.

The **overrides chip** counts everything that changes a run beyond the
project and its files: the settings (network, TLS, cookies; **Verify
certificates** off is `--insecure`), the session
overrides, the mock's `base_url`, and anything the CLI's own configuration
supplies (below).

### The CLI's configuration applies

The app honors what the CLI honors: the user config file
(`$XDG_CONFIG_HOME/hurl/config`, falling back to `$HOME/config/hurl/config`;
see [compat.md](compat.md#config-file)) and the `HURL_*` and `SONDE_*`
environment variables, with the same precedence ([architecture.md](architecture.md#variable-precedence-lowest--highest-hurl-aligned)).
They show up in the overrides chip.

A program started from the Finder, the Dock, the Start menu or a desktop
launcher does **not** inherit the environment of your shell. Variables you
export in `.zshrc` or `.bashrc` are not seen; only the app's own process
environment and the config file apply. To use them, start the app from a
terminal, or put the option in the config file or in `sonde.yaml`.

### Copy as

**Copy as** (the toolbar menu, the palette and the file tree) copies:

- a `curl` command for the request at the cursor or for the whole file, the
  way `sonde export curl` renders it, with the file's captures from its last
  run filled in;
- a `sonde` command that reproduces the run: run this file, run to a request,
  the Send you just did, or a test run.

The `sonde` command is the app's run, written out. It carries `--file-root .`
(the project folder, as the app confines paths), `--env` when one is chosen,
and one flag for each override: `--proxy`, `--connect-timeout`, `--retry`,
`--cacert`, `--cert`, `--key`, `-b` with the kept jar, and a `--variable name=value`
per session override and for the mock's `base_url`. Secrets appear as
variable references, never as values. It is quoted for your platform's
shell (POSIX or PowerShell). Run it in the project folder and you get the
same requests; the app's tests run the copied command with the CLI and
compare.

Variables the app inherits as `SONDE_VARIABLE_*` go into the `sonde` command
as `--variable` flags, so that it reproduces the run where those variables
are not set. A variable that looks like a credential (its name contains
token, secret, key, password, session, auth, cookie, jwt or bearer, or its
value has the shape of a JWT) is left out and **named in the note** under the
command: set it in CI yourself.

After a Send, the note says which earlier run's captures it reused and what
the command runs: "Send reused captures from the run at HH:MM; this command
runs 1–N".

By default every credential in the copied text is masked. Hold **⌥** while
choosing the copy to copy with the real values. A confirmation says what
goes to the clipboard. The window app only: server mode has no reveal. The
text goes from the app's Go side to the system clipboard marked concealed
(macOS) or excluded from clipboard history (Windows), and is cleared after
60 seconds if the clipboard still holds it. The page never sees the value.
Paste it only where you would type the secret.

## Request files from other tools

**Import** (palette: "Import curl command…", "Import Postman collection…",
"Import Bruno / OpenCollection…", "Import .http file…", "Import OpenAPI
spec…") calls the same converters as `sonde import`, with the same options,
so the files written are the command's, byte for byte
([guides/import-export.md](guides/import-export.md)). A preview lists the
files first. Files that already exist are overwritten only if you tick
them.

- **Where it writes.** Collections default to a folder named `imported`.
  Because that folder gets its own `sonde.yaml` (as `sonde import -o imported`
  does), the result says that the app uses the project's own `sonde.yaml`
  and where the environments went. Into the project folder itself, an
  existing `sonde.yaml` is never replaced and the result says the
  environments were not added. A curl command goes to the project folder, or
  is inserted into the open file.
- **curl secret lifting.** A credential typed into a pasted command (an
  `Authorization` header, the password of `-u`, an API key header or query
  value) is offered as a secret: ticked by default, it is replaced with a
  `{{name}}` reference and its value is written to the chosen environment's
  secrets file (mode 0600). Names avoid ones the environment already uses.
  Lifting needs an environment; without one the credential stays in the file
  and the result says so. The preview shows the references, not the values.
- **Postman suggestions.** After a Postman import the app can suggest asserts
  and a login request, derived from the collection's test scripts and OAuth 2
  settings by text patterns. Scripts are never run. Each suggestion is a
  change of its own (an assert, a capture, a login request) that you accept,
  reject or undo; Apply writes only the accepted ones, and the result is
  parsed before it is written. Suggestions are optional and apply only to
  files as the import wrote them.

Imports take up to 64 MiB per input.

## History and privacy

Finished runs are kept per project in the app's config folder (`history/`,
mode 0600, in the shape of the CLI's JSON report). The History panel lists
their requests, newest first (method, path, status, file); opening one shows
its run in the results panel, read-only, on that request. Nothing leaves the
computer.

Settings › History & privacy turns history off, or keeps runs for 7 days, 30
days (the default) or forever, and clears it. Bodies are not stored, and
neither are curl commands (they repeat static credential headers).

What is masked in the stored runs, as in everything the app shows:

- every secret of the run: declared secrets, `redact` captures, data-file
  secret columns, `--secret` values;
- the values of credential headers (`Authorization`, `Proxy-Authorization`)
  and **every cookie value**, wherever it appears (`Cookie` and `Set-Cookie`
  headers, cookie lists, `[Cookies]` lines, curl `--cookie`);
- **captured tokens, found by name or shape**: a capture whose name contains
  token, secret, key, password, session, auth, cookie, jwt or bearer, or
  whose value looks like a JWT, even when you did not declare it secret.

Other captures, ids and totals stay readable. The name heuristic can miss a
secret under an ordinary name: declare it secret (Mark secret, or `redact`).

Where things live (`Sonde` under the usual folders: `~/Library/Application
Support` and `~/Library/Caches` on macOS, `%AppData%` and `%LocalAppData%` on
Windows, `~/.config` and `~/.cache` on Linux):

| Folder | Holds |
|---|---|
| config | `settings.json`, `history/`, `cookies/` (kept jars, real values), `trust.json` (git folder trust), an env-edit journal |
| cache | spilled response bodies, staged import inputs, the server mode launch link |

Both folders are private to your user (0700). `--data DIR` puts them under
`DIR/config` and `DIR/cache` instead.

## Git and folder trust

The Files panel shows the branch, the changed files and a Commit action. A
repository's own configuration can make git run programs (filter drivers,
fsmonitor, hooks). So a folder is **untrusted** until you trust it:

- the branch is read from `.git/HEAD` without running git;
- status and commit appear only after you choose Trust for that folder
  (remembered in `trust.json`; "Not now" is remembered per folder);
- every git run disables fsmonitor and hooks, ignores the system config,
  never prompts, and is stopped after 30 seconds. A commit in a trusted
  folder runs your hooks as usual; if one hangs, the app tells you to commit
  in a terminal.
- secrets files (`*.secrets` and the ones `sonde.yaml` lists) are flagged in
  the Changes card and refused by Commit.

## Responses

The results panel shows the run as it happens: each request, its timings,
the asserts and captures, and the response body in tabs (formatted, raw, a
tree for JSON, a sandboxed preview for HTML and images).

- Bodies are decoded and redacted before they reach the page, binary ones
  too. A body that cannot be decoded (a truncated gzip) is not shown at all,
  because redaction cannot see inside compressed bytes.
- A preview of HTML runs in a frame with no scripts, no network and no
  navigation, and the body is served with `X-Content-Type-Options: nosniff`.
- Bodies over 20 MB are not formatted unless you ask (the first 1 MB shows
  as raw text); over 50 MB they can only be saved or opened elsewhere. The
  asserts and captures always run on the full body.
- **Save response to file** (⌥⌘S) writes the body as received, unredacted:
  it is your data, to a file you pick. **Open in default app** writes the
  redacted body to a private temp file with an allowlisted extension
  (`.pdf`, `.png`, `.jpg`, `.gif`, `.txt`, `.json`, `.xml`; anything else,
  HTML included, `.txt`), marks it as downloaded (quarantine on macOS, Mark
  of the Web on Windows) and opens it. The temp files are removed when the
  app quits. Both are window-app features.
- WebSocket entries can be opened as an interactive session; its messages are
  redacted, never stored, and can be written back to the file as steps.

## Server mode

`sonde-desktop-server` serves the same app to a browser, for a project on a
machine you reach over the network, such as a remote dev box. It is a
separate, CGO-free binary. Download `Sonde-Desktop-Server-X.Y.Z-OS-ARCH`
from the release (for a Linux dev box, usually `linux-amd64` or
`linux-arm64`), check it as above, and `chmod +x` it. These binaries are not
signed or notarized. If a browser downloaded one to a Mac, macOS may refuse
to run it until you clear the quarantine mark with
`xattr -d com.apple.quarantine FILE`; one fetched with `curl` or `scp` has
no such mark. To build it yourself, run `make desktop-tools`, then
`../bin/task build:server` in `desktop/`, which gives
`desktop/bin/sonde-desktop-server`. The examples below call it
`sonde-desktop-server`.

```sh
sonde-desktop-server --root DIR [--port N] [--data DIR] [--open]
```

| Flag | Meaning |
|---|---|
| `--root DIR` | the project folder (default `.`) |
| `--port N` | the port on 127.0.0.1; 0, the default, picks a free one |
| `--data DIR` | app data in `DIR/config` and `DIR/cache` instead of the user folders |
| `--open` | open the launch file in the default browser |

It listens on `127.0.0.1` only; there is no flag to change that. It prints
the address, never a secret, then writes a **launch file**, a small HTML page
(mode 0600) at `launch-<port>.html` in the cache folder, once the port is
listening. The page holds a link with a one-time nonce:

- the link works once and only for 60 seconds; press Enter in the terminal
  for a new one, which revokes the old one;
- the file is deleted when its link is used, when it expires and when the
  server stops;
- the nonce is traded for an `HttpOnly`, `SameSite=Strict` session cookie,
  and the page then reads a per-launch token and sends it as the
  `X-Sonde-Token` header on every call. A page or process with the cookie
  but not the token is refused.

Every request is checked: `Host` must be `127.0.0.1:<port>` or
`localhost:<port>`; `Origin`, when present, must be this server, and
requests other than GET and HEAD must send it; a cross-site or same-site
`Sec-Fetch-Site` is refused. No response can be framed, except that the app
frames its own sandboxed body previews. No response carries CORS headers.

### On a remote dev box

Run the server on the remote machine, and forward the **same port number**
to your computer (the `Host` check needs it to match):

```sh
# on the remote machine
sonde-desktop-server --root ~/projects/shop-api --port 34567

# on your computer
ssh -L 34567:127.0.0.1:34567 devbox
```

Copy the link from the launch file on the remote machine
(`cat ~/.cache/Sonde/launch-34567.html`, or the path the server printed),
and open it in a browser on your computer. If it expired, press Enter in the
server's terminal for a new one. Requests are sent from the remote machine,
so it is the one that must reach the API under test. A sandboxed browser that
cannot read the cache folder, such as Firefox from snap on Ubuntu, needs the
link copied from the file.

### What server mode leaves out

Server mode registers fewer bindings than the window: nothing that needs a
native dialog, a path on the machine of the person at the screen, or the
clipboard of the server. The window app alone has:

- native file dialogs: the data-file run, the TLS files in Settings,
  picking a file for a body, **Open a folder** (use `--root`);
- Reveal in Finder / Explorer and Move to Trash;
- Save response to file and Open in default app;
- report export;
- Copy as with real secret values (no reveal).

Imports work in server mode: the page uploads the file instead (up to 64
MiB). The server also does not use Wails' own event socket, which accepts
rebinding pages; app events go through a guarded stream.

## Appearance and themes

Settings › General › Theme has two modes:

- **Sync with system** (the default): the **Day theme** is used while the
  system is light, the **Night theme** while it is dark. Each has a preview
  card; the one in use is marked Active, and it switches as soon as the
  system does.
- **Manual**: one theme, always. Choosing Manual keeps the theme in effect.

The built-in themes are Light, Solarized Light, Ayu Light, GitHub Light,
Catppuccin Latte, Tokyo Night Light, VS Code Light, Gruvbox Light and Rosé
Pine Dawn; Dark, High Contrast Dark, Ayu Dark, Dracula, Monokai, Night Owl,
Solarized Dark, GitHub Dark, Catppuccin Mocha, Tokyo Night, VS Code Dark,
One Dark Pro, Nord, Gruvbox Dark and Rosé Pine. Their colors are adjusted
where needed so text and run results stay readable (WCAG contrast); syntax
colors keep each theme's own.

**Select theme…** in the palette lists them all: the highlighted theme
shows at once, typing filters the list, Enter keeps it and Esc puts the
theme back. With Manual, Enter sets the theme; with Sync, it sets the slot
the system uses now (the Night theme on a dark system) and Sync stays on.

The rail's theme button and **Toggle day / night theme** switch between
the Day and Night themes: with Sync they switch to Manual with the other
slot's theme; with Manual they go from a dark theme to the Day theme and
from a light one to the Night theme.

The keys in `settings.json` are `appearance.theme` (`"system"` or a theme
id), `appearance.dayTheme` and `appearance.nightTheme`; an unknown id (from
a later version) is read as the default. The page opens in the theme
already, with no flash of another one, in the window and in server mode.
Native menus and dialogs follow the system's look, not the app's theme.

## Shortcuts

`⌘` is Ctrl on Windows and Linux. The shortcuts sheet (`?`) lists them and
lets you rebind them; changes are saved in `settings.json`.

| Keys | Command |
|---|---|
| ⌘↵ | Send request at cursor |
| ⇧⌘↵ | Run requests up to the cursor |
| ⌘R | Run file |
| ⌘S | Save file |
| ⌥⌘S | Save response to file (window app) |
| ⌘K | Search files and commands (the palette) |
| ⌘E | Switch Text / Form |
| ⇧⌘F | Filter requests in all files |
| ⌘/ | Toggle comment (while the editor has the focus) |
| ⌘G | Go to line |
| ⌘O | Open a folder (window app) |
| ⌘I | Import… (the Import dialog; in the window with no folder open it asks for one first) |
| ? | Keyboard shortcuts |

The app's shortcuts win over the editor's own in the text view. Only
combinations with ⌘ or Ctrl, or with ⌥, or the F1 to F12 keys, or `?` can
be bound. ⌘R does not reload the page: the window app's menu has no Reload.

## Limits

Only what the code enforces:

| What | Limit |
|---|---|
| Import input (a file or pasted text) | 64 MiB |
| A decoded response body | 512 MiB by default (`max-filesize` changes it), as in the CLI |
| Redacted bodies kept in memory | 512 MiB, then the oldest spill to the cache folder; dropped beyond 2 GiB on disk |
| A body shown in the panel | formatted up to 20 MB, shown (raw) up to 50 MB |
| Language server message | 64 MiB |
| Language server session | ends after 3 minutes without a heartbeat from the page |
| A native-dialog selection | valid once, for 5 minutes |
| A git command | 30 seconds |
| Server mode launch link | one use, 60 seconds |
| Server mode event stream | 32 MiB per client; a page that lags is dropped and reconnects |
| Runs | one at a time per file |
| File watching | polled every 2 seconds in projects over 2000 entries |

## The window app's flags

```sh
sonde-desktop [--root DIR] [--data DIR] [--perf-trace FILE [--perf-tour JSON]]
```

| Flag | Meaning |
|---|---|
| `--root DIR` | open this project folder at start; it must exist |
| `--data DIR` | app data in `DIR/config` and `DIR/cache` instead of the user folders |
| `--perf-trace FILE` | run the performance tour, write the measures to FILE as JSON, then quit |
| `--perf-tour JSON` | the tour's parameters; `scripts/perf.mjs` passes them |

`--perf-trace` and `--perf-tour` are for measuring a build, not for
everyday use. `node scripts/perf.mjs`, run in `desktop/` after packaging,
launches the app on a generated 1000-file project and prints the budget
table; see [desktop/MANUAL-TEST.md](../desktop/MANUAL-TEST.md). They are
flags and not `SONDE_*` variables because that namespace is read by the
run planner.
