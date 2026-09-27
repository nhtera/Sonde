<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde lsp

Run the language server over stdio.

```
sonde lsp [flags]
```

Lsp runs a Language Server Protocol server for .hurl and .sonde files on
stdin/stdout: diagnostics, completion, hover and formatting. Editors start
it themselves; see docs/guides/editors.md. It never sends HTTP requests.

## Flags

```
      --stdio   use stdin/stdout (the only transport) (default true)
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde](sonde.md)
