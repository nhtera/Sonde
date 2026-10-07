# Migrating from Hurl

Sonde runs Hurl 8.0.1 files as they are and accepts Hurl's command line, so
most projects switch by replacing the command name. This guide lists what
stays the same, what maps to something else, and what Sonde adds. Every known
difference is in [compat.md](../compat.md).

## What stays the same

- **Files.** `.hurl` files run unchanged, and `sonde fmt` keeps them valid
  for Hurl. `.sonde` files use the same syntax in v1.
- **Flags.** Every Hurl flag has the same name and meaning. A few are
  accepted but unsupported and fail with a clear error: `--aws-sigv4`,
  `--digest`, `--ntlm`, `--negotiate`, `--http1.0`, `--http3` and
  `--ssl-no-revoke`. See [compat.md](../compat.md#cli-flags).
- **Environment variables.** `HURL_*` variables work, including
  `HURL_VARIABLE_name` and `HURL_SECRET_name`. Each also has a `SONDE_*`
  form, which wins when both are set.
- **Config file.** Sonde reads `$XDG_CONFIG_HOME/hurl/config` (or
  `~/.config/hurl/config`) with every key of the reference implementation,
  and so does the desktop app. See [compat.md](../compat.md#config-file).
- **Exit codes.** They are the same, plus `130` when the run is interrupted
  with Ctrl-C.

## Commands

| Hurl | Sonde |
|---|---|
| `hurl FILE…` | `sonde FILE…` or `sonde run FILE…` |
| `hurl --test FILE…` | `sonde --test FILE…` or `sonde test FILE…` |
| `hurlfmt --check FILE` | `sonde fmt --check FILE` (same output; exit 1, not 3, when a file is not formatted) |
| `hurlfmt --in-place FILE` | `sonde fmt --write FILE` (same layout, section order included) |
| (a syntax check) | `sonde check FILE` (exit 2 on a syntax error) |
| `hurlfmt --in curl` | `sonde import curl INPUT -o DIR` (options Sonde cannot send yet, such as `--digest`, are kept with a warning) |
| `hurlfmt --out curl FILE` | `sonde export curl FILE` |
| `hurlfmt --out json` / `--out html [--standalone]` | `sonde export json FILE` / `sonde export html [--standalone] FILE` (same output, see [compat.md](../compat.md)) |
| `hurl --curl FILE` | `sonde --curl FILE` (same format; secrets redacted) |

## What behaves differently

The table below lists the differences you are most likely to notice.
[compat.md](../compat.md) has the full list.

| Area | Hurl | Sonde |
|---|---|---|
| Default `User-Agent` | `hurl/<version>` | `sonde/<version>` (`SONDE_DEFAULT_USER_AGENT` replaces it) |
| Secrets in `--json` | printed | redacted (`***`), as in every other output |
| File paths in `[Options]` (`cacert`, `cert`, `key`, …) | any path | confined to the file root |
| A symbolic link leaving the file root | followed | denied |
| Response body without `--max-filesize` | unlimited | 512 MiB |
| HTTP/3 | when libcurl supports it | not supported |

## What Sonde adds

- **`sonde.yaml` environments.** Named sets of variables and secrets files,
  selected with `--env` ([sonde-yaml.md](../sonde-yaml.md)).
- **OpenAPI contracts.** `--openapi spec.yaml` checks every response
  against a spec ([openapi.md](openapi.md)).
- **Data-driven runs.** `--data rows.csv` runs a file once per row
  ([data-driven.md](data-driven.md)).
- **Importers.** `sonde import` reads curl commands, Postman collections,
  OpenCollection files, `.http` files and OpenAPI specs
  ([import-export.md](import-export.md)).

## Running both side by side

The conformance harness runs Hurl's own test scripts with a `hurl` wrapper
that calls Sonde ([conformance.md](../conformance.md)). You can do the same
to check a project before switching. Put a script named `hurl` first on
`PATH` that runs `exec sonde "$@"`, then run your existing CI script.
