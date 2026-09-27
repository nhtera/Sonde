# Streaming: Server-Sent Events and WebSocket

Sonde tests streaming APIs in `.sonde` files. An entry can:

- read a `text/event-stream` response as Server-Sent Events (SSE);
- open a WebSocket, send messages, and wait for the server's replies.

The received messages are checked with the usual `[Asserts]` and
`[Captures]`, through one query, `sondeStream`.

These constructs exist only in `.sonde` files. In a `.hurl` file each one
is a parse error that names it, so a `.hurl` file always stays valid for
Hurl itself ([file-format.md](../file-format.md)). The design is recorded
in [decisions/0004-streaming-protocols.md](../decisions/0004-streaming-protocols.md).

## Server-Sent Events

Any `sonde-stream-*` option makes Sonde read the response body as an
event stream, and stop at the first limit reached:

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

| Option | Default | Stops the stream when |
|---|---|---|
| `sonde-stream-count: N` | no limit | N events have arrived |
| `sonde-stream-timeout: DURATION` | `10s` | this much time has passed since the response headers |
| `sonde-stream-max-bytes: N` | `10485760` (10 MiB) | N body bytes have been read, after decompression (with a warning) |

The stream also stops when the server closes it.

A limit is not a failure: the asserts decide. When a stream should have
sent 3 events but the timeout stopped it after 2, `sondeStream count == 3`
fails and shows the actual count.

- A duration without a unit is in milliseconds, as for `max-time`, and
  `0` keeps the default.
- `max-time` (or `--max-time`) still limits the whole entry, and exceeding
  it is an error. The events read before it are kept in the results.
- A compressed stream (`Content-Encoding: gzip`, `br`, `deflate`, `zstd`)
  is decoded as it arrives. The `body` query returns the bytes as
  received.
- Reconnection is not followed: a test observes one connection, and
  `retry` and `Last-Event-ID` are not acted on.

Without a `sonde-stream-*` option, the body is read to the end as usual.
`sondeStream` then parses it as an event stream, which suits a server that
closes the stream by itself.

Parsing follows the [WHATWG algorithm](https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation):

- `data` lines are joined with a newline;
- `:` comments are ignored;
- an event without `data` is not dispatched;
- an `id` containing NUL is ignored;
- a `retry` that is not all digits is ignored.

## WebSocket

A `[SondeMessages]` section makes the entry a WebSocket exchange. Its steps
run in order:

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
```

| Step | Meaning |
|---|---|
| `send: MESSAGE` | Send one message. JSON (which may span lines), a `` `text` `` string, a ```` ``` ```` multiline string or XML is sent as a text frame. `base64,…;`, `hex,…;` or `file,…;` is sent as a binary frame. Templates are rendered as in a request body. |
| `receive` | Wait for the next message from the server; `receive: N` waits for the next N. |
| `close` | Send a close frame (code `1000`, or `close: CODE`) and wait for the server's reply. |

A plain word is not a message: write ``send: `hello` ``, not `send: hello`.
A message is never cut at `#`, so `send: {"color": "#fff"}` sends the
whole JSON.

### The handshake

- The URL is `ws://`, `wss://`, `http://` or `https://`. The method must be
  `GET`, and the entry has no request body, `[Form]` or `[Multipart]`.
- The handshake uses the entry's headers, cookies, credentials, TLS and
  proxy options. It always uses HTTP/1.1: the `http2` and `http3` options
  are ignored, with a warning.
- Its response is the entry's response, so `HTTP 101` checks the upgrade
  and headers can be asserted.
- When the server refuses the upgrade, for example with `401`, no step
  runs, and the `HTTP` line checks that status. The response body is kept
  whole.
- The response has no IP address and no phase timings, so the `ip` query
  returns nothing.
- A `ws://` or `wss://` URL without `[SondeMessages]` is an error.

### Timing and failures

`sonde-stream-timeout` (default `10s`) limits the whole exchange after the
handshake. Unlike an event stream, a WebSocket script states what it
expects, so these fail the entry at the step that was running:

| Failure | Message |
|---|---|
| A `receive` still waiting at the timeout | `timeout waiting for message 2 of 3` |
| A `receive` still waiting when `max-time` ends the entry | `timeout (max-time) waiting for message 2 of 3` |
| The server closes during a `receive` | `the server closed the connection (code 1000) while waiting for message 1 of 1` |
| All the messages received, taken by a step or not, exceed `sonde-stream-max-bytes` | `more than N bytes received` |

- The server may send before a `receive` asks: its messages are queued in
  order. Messages still queued when the script ends are not part of the
  results.
- After the last step, Sonde closes the connection with code `1000`,
  unless a `close` step already did. A server that never answers the close
  frame does not hold the entry past the timeout: the connection is
  dropped.
- `retry` reconnects and replays every step.
- A value received in a step cannot be used by a later `send` of the same
  entry, because captures run after the entry. Each entry opens its own
  connection.

## The `sondeStream` query

```
sondeStream              # the data of each message received, in order
sondeStream "FIELD"      # another field of each message
```

The query returns a list, so `count`, `nth` and the other filters apply.
`nth` returns one message's data, a string that `jsonpath` can parse.

| Field | SSE | WebSocket |
|---|---|---|
| `data` (default) | the event's data | the message: a string for a text frame, bytes for a binary one |
| `event` | the event type (`message` when the event names none) | — |
| `id` | the last event ID | — |
| `retry` | the event's `retry` field as an integer, else null | — |
| `type` | — | `text` or `binary` |

A field that does not apply to the entry's protocol, such as `event` on a
WebSocket, is a runtime error. A response that was not streamed returns no
value.

## Output and reports

| Where | What is shown |
|---|---|
| `--verbose` | Every message sent (`>>`) and received (`<<`), then why the stream stopped. |
| JSON (`--json`, `--report-json`) | The whole transcript under `sonde.stream` on the entry ([report-json.md](../report-json.md)). |
| HTML report | The first 100 messages, 2 KiB each. |

All of it is redacted like any body.

`--curl` and `sonde export curl` write `curl --no-buffer` for an event
stream. curl cannot script a WebSocket: `sonde export curl` skips such an
entry with a warning, and `--curl` leaves it out.

## Out of scope

- reconnecting event streams;
- WebSocket subprotocol negotiation, beyond a `Sec-WebSocket-Protocol`
  header you set yourself;
- per-message compression;
- using a received value in a later `send` of the same entry;
- sharing a connection between entries;
- load testing.
