<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde mock

Serve a mock of an OpenAPI spec.

```
sonde mock [options] SPEC
```

Mock serves the operations of an OpenAPI 3.x (or Swagger 2.0) spec over HTTP.
Each response is the lowest documented 2xx, or the status a
`Prefer: code=NNN` header asks for; its body is the example named by
`Prefer: example=NAME`, else the first documented example, else a
minimal instance generated from the schema. The same request always
gets the same bytes. Errors are application/problem+json: 404 for an
unknown path, 405 for an unknown method, 406 when no documented media
type matches Accept. It binds 127.0.0.1 unless --host says otherwise,
logs one line per request to stderr and stops on Ctrl-C or SIGTERM.
See docs/guides/mock-server.md.

## Flags

```
      --cors                   answer CORS preflights and allow any origin
      --host string            address to bind (0.0.0.0 listens on every interface) (default "127.0.0.1")
      --openapi-allow-remote   allows a remote spec and remote $ref targets
      --port int               port to listen on (0 picks a free one) (default 4010)
      --server string          serve under this server URL's base path instead of the spec's servers
      --validate-requests      answer requests that do not match the spec with 415/422 problems
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde](sonde.md)
