# Mock server

`sonde mock` serves an OpenAPI description over HTTP, so a frontend or a
test suite can run against the contract before the backend exists. It
answers from the examples the spec documents, or from minimal instances
generated from its schemas. OpenAPI 3.0 and 3.1 are supported; a Swagger
2.0 document is converted on load.

```sh
sonde mock openapi.yaml
# sonde mock: serving 5 operation(s) of openapi.yaml at http://127.0.0.1:4010 (Ctrl-C to stop)
curl http://127.0.0.1:4010/v1/pets
```

The spec is read once at start: restart the mock to pick up changes.

## Routing

A request path loses the base path of the spec's servers (`/v1` for
`https://api.example.com/v1`), then goes to the path template matching it
best. Literal segments win over parameters, so `/pets/mine` beats
`/pets/{petId}`. `--server URL` uses the base path of `URL` instead:
`sonde mock openapi.yaml --server http://localhost/api` serves `/api/pets`.

A `HEAD` request without its own operation is answered by the `GET`
operation, without a body.

## Choosing the response

| What | Default | Override |
|---|---|---|
| Status | the lowest documented `2xx` (else the lowest documented status, else `200` for `default`) | `Prefer: code=404`, any code from `200` to `599`, documented exactly, by its range (`4XX`) or by `default` |
| Media type | the first of JSON, `*+json`, form, multipart, text, other | the `Accept` header, with `q` weights (`q=0` excludes a type) |
| Body | the media type's `example`, else its first `examples` entry by name, else a generated instance | `Prefer: example=NAME` picks a named `examples` entry |
| Headers | every documented response header, from its example or its schema; arrays and objects in the simple style (`a,b`) | — |

Both preferences can share a header: `Prefer: code=404, example=missing`.
When a preference is given twice, the first value wins. A quoted value may
contain commas: `Prefer: example="a, b"`.

A structured example (an object or an array) is only ever sent as JSON.
For another media type, such as `application/xml`, the mock needs a string
example. Without one it answers the next acceptable media type, or `406`
when none is left.

A generated instance is minimal and deterministic. It keeps only the
required properties, uses one array item (or `minItems`), the first
`oneOf`/`anyOf` branch, fixed values for string formats (`uuid`, `email`,
`date-time`, …) and numbers within bounds. `readOnly` properties are
kept and `writeOnly` ones left out. The same request always gets the same
bytes, whatever the order or concurrency of requests.

The generator cannot satisfy every schema. A regex `pattern`, `not`,
`multipleOf` or overlapping `oneOf` branches may yield a body its own
schema rejects. The mock still answers it and logs a warning once, naming
the operation and the reason: add an `example` to the spec to fix it.

## Errors

Requests the mock cannot answer get an RFC 9457
`application/problem+json` document:

| Status | When |
|---|---|
| `400` | `Prefer` names a status or example the operation does not document, or a code outside `200`–`599` |
| `404` | no path template matches |
| `405` | the path exists without this method (the `Allow` header lists its methods) |
| `406` | no documented media type matches `Accept`, or none of those matching has an example the mock can send as that type |
| `413` | the request body is larger than 16 MiB |
| `415` | `--validate-requests`: the request's `Content-Type` is not documented |
| `422` | `--validate-requests`: the request does not match its operation (`violations` lists why) |

```json
{"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"the request does not match POST /pets","violations":["body /name: property \"name\" is missing (required)"]}
```

## Request validation

`--validate-requests` checks each request's path, query, header and
cookie parameters and its body against the operation before answering. A
request with a body whose `Content-Type` the operation does not document
gets a `415`.
Security requirements are not enforced: any credentials, or none, are
accepted.

## Browsers

`--cors` answers every CORS preflight request with `204`, whether or not
the spec documents its path (the request that follows gets the `404` or
`405`). It also adds
`Access-Control-Allow-Origin` (the request's `Origin`, or `*`) and
`Access-Control-Expose-Headers: *` to every response. Credentials are
never allowed.

## In CI

Start the mock in the background and wait until it answers. Then point
the suite at it and check every response against the same spec:

```sh
sonde import openapi openapi.yaml -o api
sonde mock openapi.yaml --port 4010 --validate-requests &
trap 'kill $!' EXIT                           # stop the mock however the job ends
until curl -s -o /dev/null http://127.0.0.1:4010/; do sleep 0.2; done
sonde test --openapi openapi.yaml --variable base_url=http://127.0.0.1:4010/v1 api
```

Any HTTP answer, even the `404` for `/`, means the mock is serving.

`--port 0` picks a free port; the address is printed on stderr.

## Security

The mock binds `127.0.0.1` by default. Use `--host 0.0.0.0` to accept
connections from other machines. It never serves files, request bodies
are capped at 16 MiB (read only once the request is routed), and its
access log (one line per request on stderr: method, path, status,
duration) never includes query strings, headers or bodies. A remote spec, or a `$ref` to a remote document, needs
`--openapi-allow-remote`. A local `$ref` must stay inside the spec's
directory.

## Stopping

Ctrl-C stops accepting connections, lets in-flight requests finish (up to
5 seconds) and exits `130`, like every command. `SIGTERM` does the same
and exits `0`. A spec that cannot be loaded exits `1` and an address that
cannot be bound exits `3`.

## Compatibility

The statuses, problem documents, `Prefer` and `Accept` handling and exit
codes described here are part of Sonde's v1 contract
([stability.md](../stability.md)): they only change additively.

## Out of scope

Callbacks and webhooks, stateful CRUD, record/replay, hot reload and
random data are not supported.
