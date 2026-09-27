<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde import

Import request files from another format.

```
sonde import KIND INPUT
```

Import converts INPUT, in KIND's format, to Sonde request files under
--output (see docs/guides/import-export.md).

Available kinds: curl, http, openapi, opencollection, postman.

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

## Subcommands

- [sonde import curl](sonde_import_curl.md) — Convert curl command lines to a request file, one entry per command
- [sonde import http](sonde_import_http.md) — Convert a .http file (JetBrains HTTP Client or VS Code REST Client) to Sonde request files
- [sonde import openapi](sonde_import_openapi.md) — Generate request files from an OpenAPI spec, one per operation
- [sonde import opencollection](sonde_import_opencollection.md) — Import a Bruno OpenCollection YAML collection (a file or a directory)
- [sonde import postman](sonde_import_postman.md) — Convert a Postman v2.1 collection to request files

Parent command: [sonde](sonde.md)
