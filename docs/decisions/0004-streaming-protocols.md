# 0004: Streaming protocols (Server-Sent Events, WebSocket)

Status: Accepted. Date: 2026-09-28.

## Context

Phase 12 adds tests for streaming APIs: Server-Sent Events (SSE) and
WebSocket. Hurl 8 has no syntax for either, so this is Sonde's first
extension of the file format. [file-format.md](../file-format.md) and
[stability.md](../stability.md) already set the frame:

- `.hurl` files stay strict Hurl 8, so they keep running with Hurl itself.
- Extensions are allowed only in `.sonde` files, under a Sonde prefix that
  Hurl will not use.

This record fixes the syntax, the semantics and the dialect rules before any
code is written. gRPC (Phase 13) is meant to reuse them, so a second
breaking redesign has to be avoided.

### Upstream Hurl (checked 2026-09-28)

| Topic | Upstream state | Consequence here |
|---|---|---|
| WebSocket, SSE ([#1096](https://github.com/Orange-OpenSource/hurl/issues/1096), open since 2022-12) | No accepted design, no PR, no release. A maintainer favors a JetBrains HTTP Client style: a `WEBSOCKET url` request line, `===` message separators and `=== wait-for-server`. Another maintainer (2026-03): "We'll likely handle SSE first... no timeline". | Avoid every live proposal: no `WEBSOCKET` method, no `===` separators, no `[SendData]`/`[AssertReceiveData]` sections. |
| gRPC ([#3411](https://github.com/Orange-OpenSource/hurl/issues/3411)) | Active. A beta is targeted for Hurl in autumn 2026: unary calls only, a ` ```grpc ` body fence, `grpc "status"` queries, a `grpc-protoset` option. The syntax is "to be refined". | Phase 13 reuses the mechanisms of this record, but it adopts Hurl's syntax for unary gRPC if Hurl has shipped it by then (the plan's risk response). |
| Latest release | Hurl 8.0.1 (2026-04). The unreleased 8.1.0 adds no streaming syntax. | Nothing to align with yet. |


Hurl's own conformance suite already covers a `text/event-stream` response
(`tests_ok/sse`): Hurl reads it like any other body, until the server
closes the connection. Sonde passes that script today, and nothing in this
record changes it.

## Decision

Three constructs, all prefixed `sonde`, all valid only in `.sonde` files:

| Construct | Kind | Used by |
|---|---|---|
| `sonde-stream-count`, `sonde-stream-timeout`, `sonde-stream-max-bytes` | options in `[Options]` | SSE; the timeout and byte limit apply to WebSocket too |
| `[SondeMessages]` | request section | WebSocket |
| `sondeStream` | query | SSE and WebSocket |

Everything else is plain Hurl: request lines, headers, `HTTP` lines,
`[Captures]`, `[Asserts]`, filters and predicates. Messages are checked
with the usual asserts, so no second assert language is needed.

### Dialect gate

- A Sonde construct in a `.hurl` file is a **parse error**, not a warning.
  A warning would let a `.hurl` file that Hurl cannot run pass `sonde fmt`
  and CI. The messages are:
  - ``section `[SondeMessages]` requires a .sonde file``
  - ``option `sonde-stream-count` requires a .sonde file``
  - ``query `sondeStream` requires a .sonde file``

  The language server shows parse errors as diagnostics, so no separate
  LSP check is needed. In a `.hurl` file, "did you mean" suggestions never
  propose a Sonde name.
- Input read from stdin parses as `.hurl`. The flag name `--dialect` is
  reserved for adding a choice later, which would not break anything.
- The reverse direction is safe too. Hurl rejects each construct at parse
  time (unknown section, option or query), so a `.sonde` file renamed to
  `.hurl` fails loudly in Hurl, instead of running differently.
- A `.sonde` file without these constructs behaves exactly like the same
  file named `.hurl`.

### Server-Sent Events

An entry is **streamed** when it sets at least one `sonde-stream-*`
option. Sonde then reads the response body incrementally, parses it as SSE
([WHATWG HTML §9.2](https://html.spec.whatwg.org/multipage/server-sent-events.html))
and stops at the first of these limits:

| Option | Default | Stops the stream when |
|---|---|---|
| `sonde-stream-count: N` | none | N events have been dispatched |
| `sonde-stream-timeout: DURATION` | `10s` | this much time has passed since the response headers arrived |
| `sonde-stream-max-bytes: N` | `10485760` (10 MiB) | N decoded body bytes have been read |
| (none) | | the server closes the connection |

A value of `0` keeps the default (no count limit for `sonde-stream-count`).
For a streamed entry, `sonde-stream-max-bytes` replaces the usual limit on
the decoded body size. Any of these ends the stream normally; none of them
is an error. The
asserts decide whether the result is right: when 3 events were expected and
the timeout cut the stream at 2, `sondeStream count == 3` fails and shows
the actual count. Hitting the byte limit also logs a warning. `max-time`
(`--max-time`) still limits the whole entry and fails it when exceeded, as
it does today; the events read before are kept in the results, as a failed
WebSocket keeps its messages. A compressed stream (`Content-Encoding`) is decoded as it
arrives; the `body` query still sees the bytes as received.

Parsing follows the WHATWG algorithm:

- one leading byte-order mark is stripped;
- CRLF, LF and CR all end a line;
- `data` lines are joined with `\n`, and the final newline is dropped;
- an `id` containing NUL is ignored;
- a `retry` value that is not all ASCII digits is ignored;
- comment lines (starting with `:`) are ignored;
- an event without data is not dispatched.

Reconnection (`Last-Event-ID`, `retry`) is **not** followed: a test
observes one connection.

The `body` query returns the bytes read, and the status and header asserts
work as usual. Without a `sonde-stream-*` option, an entry reads its body
exactly as Hurl does.

### WebSocket

An entry with a `[SondeMessages]` section is a WebSocket exchange:

- Its method is `GET`. Its URL is `ws://` or `wss://`, or `http://` or
  `https://`, since the WebSocket handshake (RFC 6455) is an HTTP request.
  `ws` maps to `http` and `wss` to `https` for cookies and proxies.
- The handshake always uses HTTP/1.1. The `http2` and `http3` options are
  ignored, with a warning.
- The handshake reuses the entry's headers, cookies, TLS and proxy options.
- The handshake response is the entry's response, so `HTTP 101` is its
  status line, and headers can be asserted and captured.
- If the upgrade is refused (for example with a `401`), the steps do not
  run. The status assert reports the real status, and the response body is
  kept whole.
- The method must be `GET` and the entry has no body (`[Form]`,
  `[Multipart]` or a body): otherwise a runtime error names the problem.
  Being a runtime error keeps the grammar open to a future protocol that
  needs a body.
- The handshake response has no IP address and no phase timings.

The section holds ordered steps:

| Step | Meaning |
|---|---|
| `send: BODY` | Send one message. `BODY` is any request-body form of Hurl: JSON, which may span lines; a `` `…` `` string; a ```` ``` ```` multiline string; XML; `base64,…;`; `hex,…;`; or `file,…;`. The first four forms send a text frame and the last three a binary frame. Templates are rendered as in a body. |
| `receive` / `receive: N` | Wait for the next 1 (or N) messages from the server. |
| `close` / `close: CODE` | Send a close frame (default `1000`) and wait for the server's. The code is WebSocket-only; gRPC will use a bare `close` for a half-close. |

`send`, `receive` and `close` are the only step names. Any other name is a
parse error. A plain word is not a valid `BODY`, so `send: hello` is
rejected; write ``send: `hello` `` instead. Unlike the value of a header,
a body form never stops at `#`, so `send: {"color": "#fff"}` sends all of
the JSON.

- After the last step, Sonde closes with `1000` unless a `close` step
  already did. The close handshake is bounded like the rest of the
  exchange: when the timeout, `max-time` or an interrupt ends it, the
  connection is dropped.
- Messages the server sends before a `receive` asks for them are queued,
  not lost. `sonde-stream-max-bytes` bounds the bytes of every message
  received, taken by a step or not: beyond it, the exchange fails at the
  `receive` that would need the next message. Messages still queued when the script ends are
  dropped; only the messages `receive` steps took are the entry's items.
- `sonde-stream-timeout` limits the whole exchange. A `receive` still
  waiting when it expires fails the entry with a runtime error (kind
  `stream`) located at that step, for example `timeout waiting for message
  2 of 3`; when `max-time` ends it, `timeout (max-time) waiting for message
  2 of 3`. A WebSocket
  script states what it expects, whereas an SSE stream only has limits.
  `sonde-stream-count` does not apply to WebSocket: `receive: N` does that
  job. The phase plan had a timeout per step; this record uses one timeout
  for the whole exchange, which is simpler to reason about and to set.
- A server close during a `receive` fails the entry, with the close code in
  the message.
- A `ws://` or `wss://` URL without `[SondeMessages]` is a runtime error
  that names the section.
- The phase plan's `receive: jsonpath "$.type" == "pong"`, with an assert
  inside the step, is dropped. Received messages are checked in
  `[Asserts]`, so there is only one assert language.

A value received in a step cannot be used by a later `send` of the same
entry, because captures run after the entry completes. Allowing a capture
inside a `receive` step would be additive later. Each entry opens its own
connection: two entries never share a WebSocket.

### The `sondeStream` query

```
sondeStream              # the data of each item, in order: a list of strings
sondeStream "FIELD"      # one field of each item, in order
```

| Protocol | Items | Fields |
|---|---|---|
| SSE | dispatched events | `data` (default), `event` (`message` when absent), `id` (the last event ID), `retry` (the event's own valid `retry` field as an integer, else null) |
| WebSocket | messages taken by `receive` steps (not sent) | `data` (default; a string for a text frame, bytes for a binary one), `type` (`text` or `binary`) |

`FIELD` is one of `data`, `event`, `id`, `retry` and `type`, checked at
parse time (with "did you mean"). A field that does not apply to the
entry's protocol, such as `event` on a WebSocket, is a runtime error. New
protocols add fields; they never rename one.

The query returns a list, so the existing filters work unchanged: `count`,
`nth`, then `jsonpath` on the string `nth` selects. On an entry that is not
a stream, `sondeStream` parses the whole body as SSE. That makes it usable
against a server that closes the stream itself, like Hurl's own SSE test.

### Transport, events, results and reports

- `internal/stream` holds the SSE parser, the SSE reader and the WebSocket
  client ([`github.com/coder/websocket`](https://github.com/coder/websocket),
  ISC, pure Go). `internal/httpx` exports the WebSocket handshake (its
  dialer, TLS config, proxy and cookie jar) and gains a hook that hands the
  decoded body of a streamed entry's final response to the SSE reader;
  every other entry takes the unchanged path. `internal/codec` holds the
  content-coding readers, shared with the decoding of whole bodies.
- `engine` gains `MessageSent` and `MessageReceived` events.
  `exchange.Response` gains a `Stream` field that is nil for other entries.
  Both are additive under [stability.md](../stability.md) and checked by
  `make apicheck`. The shape does not depend on the protocol, so gRPC fits
  it without a change:

  ```go
  type Stream struct {
      Protocol   string // "sse", "websocket"; "grpc" in a later release
      StopReason string // "count", "timeout", "max-bytes", "closed", "script"
      Messages   []Message
  }
  type Message struct {
      Direction Direction     // Sent or Received
      Binary    bool
      Data      []byte
      Event, ID string        // SSE only
      Retry     *int          // SSE only
      At        time.Duration // since the response headers
  }
  ```

  The events carry the entry index and the `Message`, and are sent as each
  message goes out or comes in.
- The retry loop wraps the whole entry: a retried WebSocket entry reconnects
  and replays every step.
- Reports:
  - The JSON report adds `sonde.stream` on the entry: protocol (a string
    value: `sse`, `websocket`, later `grpc`), why the stream stopped, item
    counts, and the items. Sent messages are included
    too for WebSocket. The data follows
    [report-json.md](../report-json.md)'s additive rule.
  - The HTML report shows the transcript, cut to its first 100 messages
    and 2 KiB per message. JUnit and TAP keep their failure messages: a
    failed step is reported there like any runtime error.
  - Message data goes through the same redaction as bodies.
- Debug output (`--verbose`, `--very-verbose`) logs each message sent and
  received, redacted.
- `sonde export curl` and `--curl` write `curl --no-buffer` for an SSE
  entry. curl has no WebSocket scripting: `sonde export curl` skips a
  WebSocket entry with a warning, and `--curl` leaves it out.

### Formatter and editor support

- The printer keeps `[SondeMessages]` steps in order, one per line. The
  formatter normalizes `key: value` spacing as it does for other sections.
- The docs table (`internal/docs/table.yaml`) gets a separate `sonde`
  group. The Hurl inventory test stays an exact match of Hurl's own grammar.
- The language server completes and explains the new section, options and
  query, but only in `.sonde` files.

## Examples

**1. SSE: the first events of a feed**

```
GET {{base_url}}/events
Accept: text/event-stream
[Options]
sonde-stream-count: 3
HTTP 200
Content-Type: text/event-stream
[Asserts]
sondeStream count == 3
sondeStream "event" nth 0 == "ready"
sondeStream nth 1 jsonpath "$.progress" == 50
```

**2. SSE: a job that finishes within 30 seconds**

```
POST {{base_url}}/jobs
{"input": "report.csv"}
HTTP 202
[Captures]
job_id: jsonpath "$.id"

GET {{base_url}}/jobs/{{job_id}}/events
[Options]
sonde-stream-timeout: 30s
HTTP 200
[Asserts]
sondeStream "event" contains "done"
sondeStream nth -1 jsonpath "$.status" == "succeeded"
```

**3. WebSocket: ping and pong**

```
GET ws://{{host}}/ws
[SondeMessages]
send: {"type": "ping", "color": "#fff"}
send: `hello`
receive: 2
HTTP 101
[Asserts]
sondeStream nth 0 jsonpath "$.type" == "pong"
sondeStream nth 1 == "hello"
```

**4. WebSocket: authenticated subscription**

```
GET wss://{{host}}/v1/stream
Authorization: Bearer {{token}}
[Options]
sonde-stream-timeout: 5s
[SondeMessages]
send: {"op": "subscribe", "channel": "orders"}
receive: 2
close
HTTP 101
[Captures]
first_order: sondeStream nth 1 jsonpath "$.order.id"
[Asserts]
sondeStream count == 2
sondeStream nth 0 jsonpath "$.op" == "subscribed"
sondeStream nth 1 jsonpath "$.channel" == "orders"
```

**5. WebSocket: binary frames and a refused upgrade**

```
GET ws://{{host}}/echo
[SondeMessages]
send: hex,00ff10;
receive
HTTP 101
[Asserts]
sondeStream "type" nth 0 == "binary"
sondeStream nth 0 == hex,00ff10;

GET ws://{{host}}/private
[SondeMessages]
receive
HTTP 401
```

In the second entry the upgrade is refused, so the `receive` step never
runs, and the `HTTP 401` line is what passes.

## Consequences

- The `.sonde` extension changes behavior for the first time, and the dialect
  now reaches the parser. [file-format.md](../file-format.md) is updated:
  "identical to `.hurl` in v1" becomes "a superset of `.hurl`".
- The new names are part of the v1 contract once released, so they can only
  change additively. That includes the default limits and the parse error
  messages.
- `internal/stream` is the only package allowed to import the WebSocket
  library. The existing depguard rule already permits `net/http` there.
- Out of scope:
  - reconnecting SSE clients;
  - WebSocket subprotocol negotiation beyond a `Sec-WebSocket-Protocol`
    header the user sets;
  - per-message compression;
  - capturing inside a step;
  - sharing a connection across entries;
  - load or soak testing.
- Revisit when Hurl ships SSE or WebSocket syntax. In `.hurl`, Sonde then
  adopts Hurl's syntax (that improves compatibility, which v1 allows). The
  Sonde constructs keep working in `.sonde`.
