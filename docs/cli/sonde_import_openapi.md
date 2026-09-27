<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde import openapi

Generate request files from an OpenAPI spec, one per operation.

```
sonde import openapi INPUT [flags]
```

## Flags

```
      --base-url-var string    variable prefixing every URL (default "base_url")
      --dry-run                print the planned files and warnings; write nothing
      --ext string             generated file extension: "hurl" or "sonde" (default "hurl")
      --force                  overwrite existing files in the output directory
      --group string           lays files out by first tag (tag), first path segment (path) or in one directory (flat) (default "tag")
      --openapi-allow-remote   allows a remote spec and remote $ref targets
  -o, --output string          output directory (required)
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde import](sonde_import.md)
