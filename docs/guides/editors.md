# Editor support

`sonde lsp` is a Language Server Protocol server over stdio for `.sonde`
files and Hurl-compatible `.hurl` files: diagnostics, completion, hover and
formatting. It never sends HTTP requests and only reads files inside the
editor's workspace folders.

## VS Code

Install the "Sonde" extension (publisher `nhtera`) from the Marketplace, or
from a `.vsix` built locally or downloaded from the `vscode` CI job's
`sonde-vscode-extension` artifact (a `sonde.vsix` file):

```sh
code --install-extension sonde.vsix
```

To build one yourself:

```sh
cd editors/vscode
npm ci
npm run compile
npx vsce package
```

The extension needs the `sonde` CLI on `PATH`, or pointed to explicitly.

### Settings

| Setting | Default | Description |
|---|---|---|
| `sonde.path` | `"sonde"` | Path to the `sonde` binary, or a command on `PATH`. Changing this restarts the language server. |
| `sonde.env` | `null` | The `sonde.yaml` environment whose variables are active. Kept in sync with the running server as you change it, without a restart. |
| `sonde.associateHurlFiles` | `false` | Run the language server on `.hurl` files already claimed by another extension under the `hurl` language id. |

Set the active environment in your workspace settings:

```json
{
  "sonde.env": "staging"
}
```

### `.hurl` files

If nothing else on your machine already claims `.hurl`, map it to Sonde's
`sonde` language id:

```json
{
  "files.associations": { "*.hurl": "sonde" }
}
```

That alone gives full support — diagnostics, completion, hover, formatting
**and** syntax highlighting — because the file now carries the `sonde`
language id, exactly like a `.sonde` file.

`sonde.associateHurlFiles` is for the other case: another extension already
owns `.hurl` under the `hurl` language id (so you want to keep its
highlighting or other features) and you additionally want Sonde's
diagnostics, completion, hover and formatting on those same files:

```json
{
  "sonde.associateHurlFiles": true
}
```

This does not change syntax highlighting, which stays whatever that other
extension provides for the `hurl` language id.

### Commands

Run **Sonde: Restart Language Server** from the command palette after
installing a new `sonde` version, or if the server needs a fresh start.

## Neovim (0.11+)

```lua
vim.filetype.add({ extension = { sonde = "sonde" } })

vim.lsp.config("sonde", {
  cmd = { "sonde", "lsp" },
  filetypes = { "sonde", "hurl" },
  root_markers = { "sonde.yaml", ".git" },
  init_options = { env = "dev" },
})

vim.lsp.enable("sonde")
```

Neovim already detects `.hurl` as filetype `hurl` on its own (built into
`filetype.lua`), so `filetypes` includes `hurl` to attach this server to
those buffers too, alongside `.sonde`. Drop it if you'd rather another
language server handle `.hurl`. `init_options.env` selects the active `sonde.yaml`
environment; omit it to use the server's default. To change the environment
at runtime, send `workspace/didChangeConfiguration` with
`{ settings = { sonde = { env = "staging" } } }` via
`vim.lsp.get_clients({ name = "sonde" })[1]:notify(...)`, or just restart
the client.

## Any other LSP client

Point your client at:

```sh
sonde lsp --stdio
```

- Transport: stdio, JSON-RPC 2.0, `Content-Length`-framed (standard LSP).
- `initializationOptions`: `{ "env": "<sonde.yaml environment>" }` (optional).
- Dynamic config: send `workspace/didChangeConfiguration` with
  `{ "settings": { "sonde": { "env": "<environment>" } } }` to change the
  active environment without restarting the server.
- Capabilities used: `textDocumentSync` (full), `completionProvider`
  (trigger characters `[` and `{`), `hoverProvider`,
  `documentFormattingProvider`. Position encoding is negotiated at
  `initialize`: the server uses `utf-8` if the client offers it in
  `capabilities.general.positionEncodings`, otherwise `utf-16`.
- File watching: the server re-stats `sonde.yaml` and every `variables_files`/
  `secrets_files` it has loaded on each request regardless, so watching is an
  optimization, not a correctness requirement. If your client supports
  `workspace.didChangeWatchedFiles.dynamicRegistration`, the server registers
  watchers itself, for `**/sonde.yaml` plus the basename of each loaded
  variables/secrets file, and updates are then reflected immediately instead
  of on the next request.

## How the server finds variables

- **`sonde.yaml` discovery.** The server looks for `sonde.yaml` starting at
  the document and walking up through its ancestors, stopping at whichever
  is closer: the document's workspace folder, or a VCS root (a directory
  containing `.git`). A symlink that resolves outside that boundary is
  ignored, and so is a `sonde.yaml` that is group- or world-writable, or not
  owned by the current user — the same insecure-file rule `sonde` the CLI
  applies.
- **Single-file mode.** A document outside every workspace folder (no
  `sonde.yaml` search is possible) gets no configuration at all: no
  environment, no captured/declared variables, and therefore no
  "undefined variable" warnings either.
- **Environment selection**, in order: the client's `sonde.env` setting
  (`initializationOptions.env`, or pushed later via
  `workspace/didChangeConfiguration`), then the server process's own
  `SONDE_ENV` environment variable, then `sonde.yaml`'s `defaults.env`. An
  empty string at any step counts as unset and falls through to the next.
- **Variable source precedence.** `HURL_VARIABLE_*`/`SONDE_VARIABLE_*` and
  `HURL_SECRET_*`/`SONDE_SECRET_*` in the server process's own environment
  outrank anything from `sonde.yaml`. Hovering a variable shows where its
  value comes from (a capture line, or which file); a secret's value is
  never shown, in hover, completion or anywhere else.
- **Caps.** Diagnostics are capped at 50 parse errors and 200 warnings per
  document, so a badly broken file doesn't flood the Problems panel.
- **Workspace folder vs. `sonde run`.** If your editor's workspace folder is
  narrower than the repository, the LSP's `sonde.yaml` search (bounded by
  that folder) can disagree with `sonde run` from a shell (which searches up
  to the VCS root): open the repository root as the workspace folder to
  match `sonde run`'s view.
- Variables from `--variable`, `--variables-file`, `--secrets-file` or a
  `--data` file's columns are only known to a `sonde run` invocation, not to
  the editor, so the server will flag them as undefined; that's expected.

## Manual smoke checklist

After installing or updating the extension or Neovim config, open a
`.sonde` file and check:

- [ ] **Diagnostics** — break a line (e.g. `GET` with no URL, or an
      `[Asserts]` line with no predicate) and confirm a red squiggle and
      message appear; fix it and confirm the diagnostic clears.
- [ ] **Completion** — type `[` at the start of a line inside a request and
      confirm section names (`Query`, `Options`, ...) are offered; type
      `{{` and confirm variable names from earlier captures are offered.
- [ ] **Hover** — hover a query name (e.g. `jsonpath`), a filter (e.g.
      `toInt`), a predicate (e.g. `contains`) or an option, and confirm
      documentation appears. Hover a variable and confirm it shows where
      the value comes from, never a secret's value.
- [ ] **Format** — run "Format Document" on a file with inconsistent
      spacing and confirm it snaps to the canonical layout (equivalent to
      `sonde fmt`).
- [ ] **Config reload** (VS Code and Neovim) — edit `sonde.yaml` to add or
      remove a variable used by an open file and confirm diagnostics update
      without reopening the file.
