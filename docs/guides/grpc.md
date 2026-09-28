# gRPC

Sonde calls gRPC services from `.sonde` files: unary and server-streaming
methods, with messages written and checked as JSON. The existing queries,
filters and predicates apply to the reply, and one query, `sondeGrpc`,
returns the call's status.

These constructs exist only in `.sonde` files. In a `.hurl` file each one is
a parse error that names it ([file-format.md](../file-format.md)). The
design is recorded in [decisions/0005-grpc.md](../decisions/0005-grpc.md).

## A call

```
POST http://localhost:50051/helloworld.Greeter/SayHello
x-request-id: {{id}}
[SondeGrpc]
proto: protos/helloworld.proto
{"name": "sonde"}
HTTP 200
[Asserts]
sondeGrpc == "OK"
jsonpath "$.message" == "Hello sonde"
```

- **The `[SondeGrpc]` section** makes the entry a gRPC call. Its lines say
  where the message types come from; empty, they come from the server.
- **The URL path** is `/package.Service/Method`. `http://` uses cleartext
  HTTP/2 (h2c), as gRPC servers expect; `https://` uses TLS.
- **The method** is always `POST`.
- **Headers** are the call's metadata. A binary value (a name ending in
  `-bin`) is written base64. Sonde sets `content-type`, `te` and the other
  `grpc-*` headers; a file may set only `grpc-timeout`.
- **The body** is the request message as JSON: a JSON body, or a string,
  multiline string or `file,…;` holding JSON. Templates render as in any
  body. Without a body, the request is the empty message.
- **The `HTTP` line** checks the real HTTP status, which is `200` for every
  call a gRPC server answered. The gRPC status is in `sondeGrpc`.

## Message types

| Line | Meaning |
|---|---|
| `proto: FILE` | A `.proto` file, compiled at run time. Repeatable. |
| `import-path: DIR` | Where `import` statements resolve. Repeatable. Default: the directory of each `proto` file. |
| `protoset: FILE` | A binary descriptor set, as written by `protoc --descriptor_set_out=FILE --include_imports` or `buf build -o FILE`. Repeatable. |

Paths are relative to the request file and confined to the file root, like
every file a request reads (`--file-root`). Google's well-known types
(`google/protobuf/timestamp.proto`…) are always available.

```
[SondeGrpc]
proto: protos/shop/v1/orders.proto
import-path: protos
```

Here `orders.proto` imports `"shop/v1/types.proto"`, found under `protos`.

With neither `proto` nor `protoset`, Sonde asks the server with **server
reflection** (`grpc.reflection.v1`, then `grpc.reflection.v1alpha`). Many
production servers disable reflection: use `proto` or `protoset` files for
those. Reflection requests carry the entry's metadata, so a server that
wants credentials for reflection gets them.

Descriptors are loaded once per run for each set of files, and once per
file and server for reflection.

## JSON

Messages use the canonical proto3 JSON mapping, as grpcurl and gRPC
gateways do:

- field names in lowerCamelCase (`user_id` is `userId`); a request may use
  either spelling;
- 64-bit integers as strings: `jsonpath "$.id" == "1152921504606846976"`;
- enums by name: `jsonpath "$.mood" == "HAPPY"`;
- `google.protobuf.Any` with its `@type`, `Timestamp` as RFC 3339,
  `Duration` as `"1.5s"`.

**Fields holding their default value (`0`, `""`, `false`, an empty list) are
omitted from replies.** Check one with `jsonpath "$.count" not exists`
rather than `== 0`. A request with an unknown field is an error.

## Status

```
sondeGrpc              # the status name: "OK", "NOT_FOUND"…
sondeGrpc "code"       # the status code: 0, 5…
sondeGrpc "message"    # the status message, "" when none
```

**A status other than OK fails the entry**, for example with
`gRPC status: NOT_FOUND: user 42 not found`, unless the entry uses
`sondeGrpc` in a capture or an assert. An entry that expects an error says
so:

```
POST http://localhost:50051/shop.v1.Orders/GetOrder
[SondeGrpc]
proto: protos/shop/v1/orders.proto
import-path: protos
{"id": "missing"}
HTTP 200
[Asserts]
sondeGrpc == "NOT_FOUND"
sondeGrpc "message" contains "missing"
```

The trailers are added to the response headers, so `header "grpc-status"`
and custom trailers can be asserted too. Without `grpc-status`, the status
comes from the HTTP status as the gRPC specification maps it (`503` is
`UNAVAILABLE`, `404` is `UNIMPLEMENTED`), from a content type that is not
gRPC (`UNKNOWN`), or from an HTTP/2 stream reset. The body of such a
response, a proxy's error page for example, is kept as received:

```
POST https://{{host}}/shop.v1.Orders/GetOrder
[SondeGrpc]
protoset: build/shop.protoset
{"id": "1"}
HTTP 503
[Asserts]
sondeGrpc == "UNAVAILABLE"
```

A server that resets the call before replying fails the entry: there is no
response to assert.

## Deadlines

`max-time` (default 300 s) is sent as the call's deadline (`grpc-timeout`),
and still ends the entry with a timeout error when it passes. To test that a
server gives up in time, set a shorter deadline yourself:

```
POST http://localhost:50051/shop.v1.Reports/Build
grpc-timeout: 200m
[Options]
max-time: 5s
[SondeGrpc]
proto: protos/shop/v1/reports.proto
{}
```

The unit is `H`, `M`, `S`, `m` (milliseconds), `u` or `n`. The server
cancels the call at the deadline, and the entry fails with
`gRPC status: DEADLINE_EXCEEDED`.

## Server streaming

Each reply of a server-streaming method is an item of `sondeStream`, as its
JSON. Without options, the stream is read until the server ends it, bounded
by `max-time` only. The `sonde-stream-*` options stop it earlier, as for an
event stream ([streaming.md](streaming.md)).

```
POST http://localhost:50051/routeguide.RouteGuide/ListFeatures
[Options]
sonde-stream-count: 3
[SondeGrpc]
proto: protos/route_guide.proto
{"lo": {"latitude": 400000000, "longitude": -750000000},
 "hi": {"latitude": 420000000, "longitude": -730000000}}
HTTP 200
[Asserts]
sondeStream count == 3
sondeStream nth 0 jsonpath "$.name" isString
```

- The body is a JSON array of the replies read, so `jsonpath "$[0].name"`
  works too.
- The status exists only when the server ended the stream. A stream stopped
  by a limit has none: `sondeGrpc` returns no value, and the entry does not
  fail for it.
- Client-streaming and bidirectional methods are not supported: an entry
  calling one fails.

## Connection options

TLS (`cacert`, `cert`, `key`, `insecure`, `pinnedpubkey`), `resolve`,
`connect-to`, `unix-socket`, `ipv4`/`ipv6`, `connect-timeout`, `delay`,
`retry`, `repeat` and `skip` work as for HTTP. `http1.0`, `http1.1` and
`http3` are errors, and redirects are not followed. An `http://` call cannot
go through an HTTP proxy (cleartext HTTP/2 does not cross one): use
`https://` or a SOCKS proxy.

## Output and reports

| Where | What is shown |
|---|---|
| `--verbose` | The request and response headers, trailers included; each reflection request; for a stream, each reply (`<<`). |
| JSON (`--json`, `--report-json`) | `sonde.grpc` (`code`, `status`, `message`) on the entry, and `sonde.stream` for a stream ([report-json.md](../report-json.md)). |
| HTML report | The status next to the response, and a stream's replies. |

Request bodies are shown as the JSON the file wrote. Everything is redacted
like any body. `--curl` leaves gRPC entries out and `sonde export curl`
skips them with a warning: curl cannot encode protobuf.

## Out of scope

- client-streaming and bidirectional methods;
- gRPC-Web;
- sending compressed requests (compressed replies are read);
- completion of service and method names in the editor.
