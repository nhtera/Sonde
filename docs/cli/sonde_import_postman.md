<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde import postman

Convert a Postman v2.1 collection to request files.

```
sonde import postman INPUT [flags]
```

## Flags

```
      --dry-run                   print the planned files and warnings; write nothing
      --environment stringArray   a Postman environment JSON file (repeatable); each becomes a sonde.yaml environment
      --ext string                generated file extension: "hurl" or "sonde" (default "hurl")
      --force                     overwrite existing files in the output directory
      --group string              lays files out one per request ("request") or one per folder, chaining its requests ("folder") (default "request")
  -o, --output string             output directory (required)
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde import](sonde_import.md)
