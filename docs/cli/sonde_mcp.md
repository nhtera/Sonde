<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde mcp

Serve request files to AI agents over MCP.

```
sonde mcp [options]
```

Mcp runs a Model Context Protocol server on stdin/stdout for AI agents
(Claude Code, Cursor, VS Code...). Its tools list the request files under
--root (sonde_list) and check them (sonde_check). With --allow-run, a
third tool runs one file (sonde_run), contacting only the hosts given
with --allow-host; request files cannot read outside the root, and
secrets are redacted from every result. It logs one line per tool call
to stderr. See docs/guides/mcp.md.

## Flags

```
      --allow-host HOST        a host sonde_run may contact: HOST, HOST:PORT, *.DOMAIN, an IP, a CIDR range, or * for any (repeatable)
      --allow-run              add the sonde_run tool (requires --allow-host)
      --env string             the sonde.yaml environment of runs that name none
      --root string            the directory the tools can see (request files cannot read outside it) (default ".")
      --run-timeout duration   the longest a run may take (1s to 10m) (default 1m0s)
      --secret NAME=VALUE      defines a secret for every run (NAME=VALUE, repeatable)
      --secrets-file FILE      sets secrets for every run from a properties FILE (repeatable)
      --variable NAME=VALUE    defines a variable for every run (NAME=VALUE, repeatable)
      --variables-file FILE    sets variables for every run from a properties FILE (repeatable)
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde](sonde.md)
