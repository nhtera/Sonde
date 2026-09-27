<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde import http

Convert a .http file (JetBrains HTTP Client or VS Code REST Client) to Sonde request files.

```
sonde import http INPUT [flags]
```

## Flags

```
      --dry-run                print the planned files and warnings; write nothing
      --env-file stringArray   a JetBrains http-client.env.json file to import as sonde.yaml environments (repeatable); a sibling *.private.env.json is read too, as a secrets stub
      --ext string             generated file extension: "hurl" or "sonde" (default "hurl")
      --force                  overwrite existing files in the output directory
  -o, --output string          output directory (required)
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde import](sonde_import.md)
