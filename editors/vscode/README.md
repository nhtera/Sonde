# Sonde

Language support for [Sonde](https://github.com/nhtera/sonde) request files (`.sonde`), including Hurl-compatible `.hurl` files, backed by the `sonde lsp` language server.

## Features

- **Diagnostics** — parse errors and semantic warnings (unknown options, undefined variables, deprecated syntax) as you type.
- **Completion** — HTTP methods, section headers, queries, filters, predicates, option names, header names, variable names and functions.
- **Hover** — documentation for queries, filters, predicates, options and functions; variable hover shows where a value comes from (secrets are never shown).
- **Formatting** — canonical layout via `sonde fmt`.
- **Syntax highlighting** — a TextMate grammar for `.sonde` files, and for `.hurl` files mapped to the `sonde` language (see below).

This extension is a thin client: it starts `sonde lsp` and talks LSP to it. It does not add run buttons or a response viewer.

## Requirements

The [`sonde`](https://github.com/nhtera/sonde) CLI must be installed and on your `PATH`, or pointed to via the `sonde.path` setting.

## Settings

| Setting | Default | Description |
|---|---|---|
| `sonde.path` | `"sonde"` | Path to the `sonde` binary, or a command on `PATH`. Changing this restarts the language server. |
| `sonde.env` | `null` | The `sonde.yaml` environment whose variables are active. Kept in sync with the running server as you change it. |
| `sonde.associateHurlFiles` | `false` | Run the language server on `.hurl` files that another extension has already claimed under the `hurl` language id. |

### Using `.hurl` files

If nothing else on your machine already owns `.hurl`, map it to Sonde's own `sonde` language id:

```json
{
  "files.associations": { "*.hurl": "sonde" }
}
```

That alone gives you diagnostics, completion, hover, formatting **and** syntax highlighting — the extension treats a `.hurl` file mapped this way exactly like a `.sonde` file, since it now carries the `sonde` language id.

`sonde.associateHurlFiles` is for the other case: you already have a `.hurl` extension installed (so `.hurl` files carry the `hurl` language id, and you want to keep its highlighting or other features) and you additionally want Sonde's language server — diagnostics, completion, hover, formatting — running on those same files. Set `sonde.associateHurlFiles: true` for that; it doesn't change syntax highlighting, which stays whatever the other extension provides for the `hurl` language id.

## Commands

- **Sonde: Restart Language Server** — stops and restarts the `sonde lsp` process, e.g. after installing a new CLI version.

## More

See [docs/guides/editors.md](https://github.com/nhtera/sonde/blob/main/docs/guides/editors.md) in the Sonde repository for the full editor setup guide, including Neovim.
