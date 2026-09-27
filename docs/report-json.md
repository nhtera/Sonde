# JSON result schema

`sonde [options] FILE... --json` prints one line of this shape per file run;
`sonde --test --report-json DIR` writes the same shape, accumulated across
every invocation, to `DIR/report.json`. One schema serves both: see
[architecture.md §5](./architecture.md#5-contracts), "JSON result contract".

The base fields below are byte-compatible with Hurl 8.0.1's own `--json`
export — a Hurl parser or dashboard built against that schema reads a
Sonde result unchanged. A `sonde` key, on a result or on an entry, holds
Sonde-only data that has no Hurl equivalent (the data row of a `--data`
run, the contract findings of `--openapi`, the messages of a streamed
entry; later gRPC); it is additive within a major version and absent when
empty.

Every string value in a result — a URL, header, cookie, capture, assert
message, curl command, and so on — has already been redacted with the
run's final secret union (`internal/redact`) and, for a `--data` run, with
the row's secrets: a secret can never appear in a report in the clear.

## `Result` (one file)

| Field | Type | Description |
|---|---|---|
| `filename` | string | The file path as given on the command line. |
| `success` | bool | Whether every attempt that was not retried passed. |
| `time` | integer | Total duration, in milliseconds. |
| `cookies` | `Cookie[]` | The cookie jar at the end of the run. |
| `entries` | `Entry[]` | One object per attempt: a retried entry appears once per attempt, a repeated entry once per repetition. |
| `sonde` | object, optional | Sonde-only data, absent when there is none. `sonde.iteration.row` (integer, 1-based) is the data row of a `--data` run; `filename` stays the file path. |

### `Cookie`

| Field | Type |
|---|---|
| `domain` | string |
| `expires` | integer (Unix seconds, 0 when session-only) |
| `https` | bool |
| `include_subdomain` | bool |
| `name` | string |
| `path` | string |
| `value` | string |

### `Entry` (one attempt)

| Field | Type | Description |
|---|---|---|
| `index` | integer | 1-based entry index in the file. |
| `line` | integer | Source line of the request method. |
| `time` | integer | Attempt duration, in milliseconds. |
| `curl_cmd` | string | The equivalent `curl` command line. |
| `calls` | `Call[]` | One HTTP exchange per call; a redirect produces more than one. |
| `captures` | `Capture[]` | Variables captured by this attempt. |
| `asserts` | `Assert[]` | One per implicit or explicit assert, in source order; with `--openapi`, each contract violation adds a failed assert at the status line. |
| `sonde` | object, optional | Sonde-only data, absent when there is none. `sonde.contract.violations` (`Violation[]`) lists the contract findings of the attempt's final response (`--openapi`); absent when it conforms. `sonde.stream` (`Stream`) holds the messages of a streamed entry (Server-Sent Events or WebSocket, `.sonde` files only). |

### `Violation`

| Field | Type | Present when |
|---|---|---|
| `kind` | string | always: `unmatched`, `status`, `header`, `content-type`, `body`, or `error` (the response could not be checked) |
| `message` | string | always |
| `operation` | string | an operation matched, e.g. `"GET /pets/{petId}"` |
| `spec_pointer` | string | the rule is located in the spec: a JSON pointer fragment, e.g. `"#/paths/~1pets/get/responses/200"` |
| `instance_path` | string | the finding is located in the response: a JSON pointer into the body (`"/0/name"`) or `"header <Name>"` |
| `warning` | bool (`true` or omitted) | the finding does not fail the entry (a request no operation matches, without `--openapi-strict`) |

### `Stream`

What a streamed entry exchanged after its response headers
([guides/streaming.md](guides/streaming.md)).

| Field | Type | Description |
|---|---|---|
| `protocol` | string | `sse` or `websocket` (later `grpc`). |
| `stop_reason` | string, optional | Why the stream ended: `count`, `timeout`, `max-bytes`, `closed` (by the server) or `script` (every WebSocket step ran). Absent when the stream failed. |
| `sent` | integer | Messages sent (WebSocket). |
| `received` | integer | Events or messages received. |
| `messages` | `StreamMessage[]` | Every message, in order. They are written inline, unlike bodies; `sonde-stream-max-bytes` bounds their size. |

### `StreamMessage`

| Field | Type | Present when |
|---|---|---|
| `direction` | string | always: `sent` or `received` |
| `data` | string | always: the text, or base64 when `binary` is set (redacted before encoding) |
| `binary` | bool (`true` or omitted) | a WebSocket binary message |
| `event` | string | an SSE event (`message` when the event names none) |
| `id` | string | an SSE event after an `id` field |
| `retry` | integer | an SSE event with a valid `retry` field |
| `time` | integer | always: milliseconds since the response headers |

### `Call`

| Field | Type |
|---|---|
| `request` | `Request` |
| `response` | `Response` |
| `timings` | `Timings` |

### `Request`

| Field | Type |
|---|---|
| `method` | string |
| `url` | string |
| `headers` | `NameValue[]` |
| `cookies` | `NameValue[]` (the request's own `Cookie` header, split into pairs) |
| `query_string` | `NameValue[]` (the URL's query parameters, decoded) |

### `Response`

| Field | Type | Description |
|---|---|---|
| `http_version` | string | `"HTTP/1.0"`, `"HTTP/1.1"`, `"HTTP/2"` or `"HTTP/3"`. |
| `status` | integer | |
| `headers` | `NameValue[]` | |
| `cookies` | `ResponseCookie[]` | Every `Set-Cookie` header, parsed. |
| `body` | string, omitted for `--json` | Present only in a `--report-json` report: the response body's path, relative to `report.json` (see "Response bodies" below). |
| `certificate` | `Certificate`, optional | The server certificate of an HTTPS response. |

### `Certificate`

| Field | Type | Description |
|---|---|---|
| `expire_date` | string | `"2028-03-16 05:18:48 UTC"` |
| `issuer` | string | e.g. `"C = US, O = Example, CN = Example CA"` |
| `serial_number` | string | lowercase hex bytes separated by `:` |
| `start_date` | string | same form as `expire_date` |
| `subject` | string | same form as `issuer` |
| `subject_alt_name` | string | e.g. `"DNS:localhost, IP Address:127.0.0.1"` |
| `value` | string | the certificate in PEM format |

### `ResponseCookie`

| Field | Type | Present when |
|---|---|---|
| `name` | string | always |
| `value` | string | always |
| `domain` | string | the `Domain` attribute was set |
| `path` | string | the `Path` attribute was set |
| `expires` | string | the `Expires` attribute was set (raw text) |
| `max_age` | string | the `Max-Age` attribute was set (raw text, not re-parsed) |
| `same_site` | string | the `SameSite` attribute was set |
| `secure` | bool (`true` or omitted) | the `Secure` flag was set |
| `httponly` | bool (`true` or omitted) | the `HttpOnly` flag was set |

### `NameValue`

| Field | Type |
|---|---|
| `name` | string |
| `value` | string |

### `Timings`

All fields are integer milliseconds, except the two timestamps, which are
UTC in `2006-01-02T15:04:05.000000Z` form.

| Field |
|---|
| `name_lookup` |
| `connect` |
| `app_connect` |
| `pre_transfer` |
| `start_transfer` |
| `total` |
| `begin_call` (timestamp) |
| `end_call` (timestamp) |

### `Capture`

| Field | Type |
|---|---|
| `name` | string |
| `value` | see "Capture value encoding" below |

### `Assert`

| Field | Type | Present when |
|---|---|---|
| `line` | integer | always |
| `success` | bool | always |
| `message` | string | `success` is `false`: the rendered failure, source snippet included |

## Capture value encoding

A captured value is encoded in the JSON type closest to it:

| Captured kind | JSON encoding |
|---|---|
| null | `null` |
| bool | `true` / `false` |
| integer that fits in 64 bits | number |
| integer too large for that | number, unquoted (arbitrary precision) |
| float | number |
| string | string |
| bytes | base64 string |
| date | its display text (e.g. `2024-01-01T00:00:00Z`) |
| regex | its source pattern, as a string |
| list | array, each element encoded the same way |
| object | `{"key": value, ...}`, member order preserved |
| XPath nodeset | `{"type": "nodeset", "size": <count>}` |
| unit (a query that matched but produced no data, e.g. a cookie flag) | `{"type": "unit"}` |
| one hop of a redirect chain | `{"status": <code>, "location": <url or "None">}` |

## Response bodies (`--report-json` only)

`--json` never embeds or references a response body. `--report-json DIR`
saves each call's response body under `DIR/store/` and points `body` at
it:

```
DIR/
├── report.json
└── store/
    ├── 5b1f2c3e-<uuid>-b6b1e9b4a5df_response.json
    ├── 8b53a2c1-<uuid>-2c7a2edc0a63_response.html
    └── ...
```

The file is named `<random-id>_response`, with an extension picked from
the response's `Content-Type` (`.json`, `.xml`, `.html`, or none); `body`
holds the path relative to `report.json`, e.g.
`"store/5b1f2c3e-<uuid>-b6b1e9b4a5df_response.json"`.

## Cumulative reports

Every `--report-*` flag accumulates: running `sonde --test --report-json
DIR ...` twice appends the second run's files to the first's in
`report.json` (and its response bodies to `store/`), rather than
overwriting it. `--report-junit` and `--report-tap` behave the same way;
see `docs/architecture.md` for their formats.
