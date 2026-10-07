<!-- Generated from internal/docs/table.yaml by `make docs`. Do not edit. -->

# Hurl Compatibility

Sonde targets Hurl 8.0.1 files and CLI behavior. This page lists every known
difference: syntax, runtime and command line.

## Syntax (`sonde check`, `sonde fmt`, all commands that read files)

Measured against the vendored Hurl 8.0.1 test tree (`testdata/conformance/hurl`):

- All 272 valid `.hurl` files outside `tests_error_parser` parse and print back
  byte for byte (also with CRLF line endings); the two intentionally invalid
  ones (invalid UTF-8, bad method) are rejected with Hurl's message.
- All 46 invalid files of `tests_error_parser` are rejected (its 3 valid
  companions parse); for the 44 with an `.err` oracle, the error position and
  the rendered message match exactly.

Differences on inputs outside the test tree:

| Input | Hurl 8.0.1 | Sonde |
|---|---|---|
| `fmt` and `export` of rare spellings: a status with leading zeros (`HTTP 000200`), an upper-case JSON exponent (`1.0E+2`), spaces inside a placeholder in a JSON body (`{{ x }}`), a count of `-0` | normalized (`200`, `1.0e+2`, `{{x}}`, `0`) | kept as written (why: normalizing them was judged not worth a rewrite of the value; kept byte for byte) |
| `export json` of a value that is not valid JSON: `nth {{i}}`, a float too large for a double, a number JSON rejects (`0e`) | written as is (invalid JSON) | written as a string, so the document stays valid JSON (why: upstream bug: invalid JSON) |
| Curl import of a `-d`/`--data` body | a ``` block, so the body gains a trailing newline | a ```raw block: the body bytes are kept exactly (why: exact bytes: a JSON body would be reformatted) |
| Curl import of `-H 'Name:'` (curl removes the header) | `Name:` header with an empty value | header removed, as curl does (why: what curl sends) |
| `[Query]` and `[QueryStringParams]` (or `[Form]`/`[FormParams]`, `[Multipart]`/`[MultipartFormData]`) in one request | only the first section is sent; `hurlfmt` drops the second | both are sent; `sonde fmt` keeps both (why: pending decision: match the reference, which sends only the first section, or keep both) |
| Curl import of `-u user:pass` | `user: "user:pass"` (the quotes are then part of the user name) | `user: user:pass` (why: same request; the option keeps the credentials on one line) |
| Regex literal errors other than an invalid `{…}` repetition | Rust `regex` message | Go `regexp` message (same position) (why: engine: Go regexp) |
| Regexes valid in only one engine, e.g. `a{,3}`, `\d{2}{2}`, `a{1001}`, `[]-Z]` | accepted | rejected (why: engine: Go regexp (RE2)) |
| … and the reverse, e.g. `[0-9[Query]` | rejected | accepted (why: engine: Go regexp (RE2)) |
| XML body with DTD-declared entities | accepted (libxml2) | rejected: `invalid XML` (why: engine: Go encoding/xml; no entity expansion) |
| XML body with text or a comment before the root element | rejected | accepted (why: engine: Go encoding/xml) |
| XML error position | character where libxml2 failed | last character read by Go's `encoding/xml` (same in all test files) (why: engine: Go encoding/xml) |
| File larger than 64 MiB | accepted | rejected: `Issue reading from FILE: file is larger than 64 MiB` (why: security limit) |

Accepted as Hurl does: text after a placeholder's variable inside a string,
e.g. `{{a b}}` or `{{a}b}}`, is ignored (and kept by `sonde fmt`); XML
declarations with any encoding or version.

`sonde fmt` and `sonde export json|html` print what `hurlfmt` prints
(checked on its suite and, file by file, on every parseable file of the
conformance suite): whitespace
normalized, sections in canonical order, `ms` added to unitless durations,
with `--color`, `--check` and `--write`. `--check` exits `1` when a file
is not formatted (`hurlfmt`: `3`), keeping sonde's documented exit codes. Curl import differences
are listed below.

## Evaluation (queries, filters, predicates, templates)

Checked against Hurl 8.0.1 by `internal/evaldiff`: 300 asserts over the
fixtures in `testdata/eval/diff` (124 of them failing on purpose, so their
error messages are compared too) give the same result and the same message,
except the rows below (`testdata/eval/diff/known-differences.txt`). Every
JSONPath, XPath, regex and date format literal of the conformance tests is
also accepted or rejected as Hurl does.

| Input | Hurl 8.0.1 | Sonde |
|---|---|---|
| Integers beyond 64 bits in comparisons, e.g. `123456789012345678901234567890 > 9223372036854775807` | compared as digit strings: false | compared numerically: true (why: upstream bug: digit strings compared) |
| Rendering of a float whose fraction is negative or below 2.2e-16, e.g. `toString` of `-1.5` or `1e-300` | `-1.5.0`, `0.000…1.0` | `-1.5`, `0.000…1` (why: upstream bug) |
| Negative JSONPath number literal with a fraction, e.g. `$[?@ == -1.5]` | read as `-0.5` | read as `-1.5` (why: upstream bug) |
| NaN (e.g. from XPath `number()` on text) compared with a number | equal to every number | equal to nothing, never less or greater (why: upstream bug: NaN equal to everything) |
| Unprefixed XPath name on an XML document with a default namespace, e.g. `count(//title)` | matches nothing (use `_:title`) | also matches the namespaced elements (why: engine: XPath library) |
| Malformed XML response body for `xpath`, or XML using entities declared in a DTD | libxml2 recovers what it can, expands the entities | `Invalid XML` error (why: engine: Go encoding/xml) |
| HTML response for `xpath` | libxml2 HTML parser | HTML5 parser: `html`, `head` and `body` always exist (why: engine: HTML5 parser) |
| `\b` in regexes, and `\W`/`\S` inside a bracket class | Unicode | ASCII (why: engine: Go regexp (RE2)) |
| Response with several content codings, e.g. `Content-Encoding: gzip, br` | decoded in listed order | decoded in reverse order (last applied first) (why: upstream bug: RFC 9110 order) |
| `Content-Encoding: zstd` | `compression zstd is not supported` | decoded (why: superset) |
| Decoded (decompressed) body larger than 512 MiB | decoded | `Decompression error` (why: security limit) |
| JSON response nested deeper than 128 levels, or with a lone surrogate escape (`\ud800`) | `Invalid JSON` | accepted (the surrogate reads as U+FFFD) (why: engine: Go encoding/json) |
| `urlQueryParam` on a malformed or unusual URL, e.g. `http:///x` | parsed by the Rust `url` crate (WHATWG rules) | parsed by Go's `net/url`: `http:///x` is `empty host`, other messages differ (why: engine: Go net/url) |
| Nested numbers compared by JSONPath `==`, e.g. `$[?@.a == [1]]` on `{"a": [1.0]}` | compared as written: not equal | compared by value: equal (why: upstream bug) |
| Date string with a leap second, e.g. `toDate` of `23:59:60` | keeps the instant at 23:59:59.999999999 | rolls over to the next minute (why: engine: Go time) |
| Regex syntax outside the common subset: `(?x)`, `\u{…}`, repetitions over 1000, class set operations `&&` `--`; `\Q…\E` | first group accepted, `\Q…\E` rejected | first group rejected (`Invalid regex`), `\Q…\E` accepted (why: engine: Go regexp (RE2)) |

Regexes (`matches`, `regex`, `replaceRegex`, JSONPath `match`/`search`) run
on Go's RE2 engine with `\d`, `\w` and `\s` translated to their Unicode
meaning, as in Hurl.

## HTTP and command line (`sonde FILE...`, `sonde run`)

Measured with the conformance harness (`make conformance`, see
`docs/conformance.md`). Differences:

| Input | Hurl 8.0.1 | Sonde |
|---|---|---|
| `http2` option on an `http://` URL | tries an h2c upgrade | HTTP/1.1 (why: no h2c upgrade: no test needs it) |
| `http3` option or `--http3` | HTTP/3, racing TCP (libcurl) | QUIC first (handshake bounded by half of `--connect-timeout`, at most 2 s), then TCP within the rest of the connect timeout when it cannot connect; never through a proxy; a host denied by `sonde mcp --allow-host` is refused, not retried over TCP (why: QUIC and TCP are tried in turn, not raced) |
| `--ssl-no-revoke` | turns off certificate revocation checks (Windows Schannel) | accepted, no effect: sonde checks no revocation on any platform (why: Go checks no revocation) |
| Order and case of response headers over HTTP/2, or through a proxy to an `https://` URL without `--http1.1` | as received | names lower-case (HTTP/2) or canonical, grouped by name and sorted (values of one name keep their order); HTTP/1.x responses otherwise keep wire order and case (why: net/http keeps HTTP/2) |
| `SONDE_HTTP1_WIRE=legacy` | not applicable | sends HTTP/1.x through Go's net/http as sonde did before its own wire layer: `--http1.0` is unsupported, response headers are sorted, and header names net/http refuses fail the request (kept for one minor release) (why: rollback for one minor release) |
| `--digest`, `--ntlm` or `--negotiate` answering a 401 challenge | the verbose log shows both exchanges | only the final exchange is logged and reported (the challenge is answered within the call, as libcurl does) (why: one call per entry, as libcurl reports it) |
| `--negotiate` credential cache | any GSS-API cache (libcurl) | a Kerberos file cache: `KRB5CCNAME` (`FILE:` caches) or `/tmp/krb5cc_<uid>`, configuration from `KRB5_CONFIG` or `/etc/krb5.conf` (why: Kerberos library) |
| The `Authorization` value a scheme computes (Digest response, NTLM and Negotiate tokens, AWS signature) | printed in verbose output | recorded as `***`: never shown in logs, `--json` or reports (sent as computed) (why: security hardening) |
| Default User-Agent | `hurl/<version>` | `sonde/<version>` (`SONDE_DEFAULT_USER_AGENT` replaces it) (why: identity) |
| `--version` output | Hurl and libcurl versions and features | sonde version, commit, build date, Go version, then a `Features:` line listing the optional transport features this build implements (e.g. `HTTP2`) (why: identity) |
| Request-file path escaping the file root through a symbolic link | allowed (lexical check) | denied (why: security hardening) |
| `cacert`, `cert`, `key`, `pinnedpubkey`, `netrc-file` in `[Options]` | any path, relative to the working directory | relative to the working directory, confined to the file root (command line values are not confined) (why: security hardening) |
| `unix-socket` in `[Options]` | any path | confined to the file root (why: security hardening) |
| Proxy environment variables | `http_proxy`, `https_proxy`, `no_proxy`, `ALL_PROXY` (libcurl) | the same, `ALL_PROXY` included, except: uppercase `HTTP_PROXY` is also read and `localhost`/loopback are never proxied (Go's convention); a SOCKS proxy from the environment goes through Go's net/http (no own HTTP/1.x layer); netrc credentials are not sent through an environment proxy unless `--netrc-allow-reroute` (why: Go's proxy rules; netrc: security hardening) |
| Response body size without `--max-filesize` | unlimited | 512 MiB (raw and decoded) (why: security limit) |
| Cookie domain matching | suffix match (`evilexample.com` matches a cookie of `example.com`) | dot-boundary match (why: security hardening) |
| `--very-verbose` connection lines (`* Connected to…`, `** …`) | libcurl debug lines | `* Connected to HOST (IP) port N` and the TLS version line only; no `**` libcurl lines (why: no libcurl) |
| `--cookie-jar` file header | `# This file was generated by Hurl` | `# This file was generated by sonde` (why: identity) |
| Secrets in `--json` output | printed as is | redacted (`***`), as in every other output (why: security hardening) |
| HTML report (`--report-html`) | per-file pages with source, timeline and waterfall | same directory layout; per-file pages with source, a timeline (each call's DNS, TCP, SSL, wait, transfer and total) with an SVG waterfall, then calls and bodies; drawn differently, no JavaScript, strict CSP (why: own report) |

## Queries

| Query | Status | Description |
|---|---|---|
| `body` | supported | the response body, decoded per Content-Encoding |
| `bytes` | supported | the decoded response body as raw bytes |
| `certificate` | supported | a field of the peer TLS certificate (Subject, Issuer, dates, serial, SAN, or the raw value) |
| `cookie` | supported | a Set-Cookie value or one of its attributes by name[Attribute] |
| `duration` | supported | total request duration in milliseconds |
| `header` | supported | the value of a named response header |
| `ip` | supported | the remote IP address the request connected to |
| `jsonpath` | supported | a JSONPath expression evaluated against a JSON body |
| `md5` | supported | the MD5 digest of the response body |
| `rawbytes` | supported | the response body as raw, undecoded bytes |
| `redirects` | supported | the list of intermediate responses in a redirect chain |
| `regex` | supported | the first capture group of a regex matched against the body |
| `sha256` | supported | the SHA-256 digest of the response body |
| `status` | supported | HTTP response status code, as an integer |
| `url` | supported | the final URL after following redirects |
| `variable` | supported | the current value of a named variable |
| `version` | supported | HTTP version of the response (HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3) |
| `xpath` | supported | an XPath expression evaluated against an HTML or XML body |

## Filters

| Filter | Status | Description |
|---|---|---|
| `base64Decode` | supported | decodes a base64 string into bytes |
| `base64Encode` | supported | encodes bytes as a base64 string |
| `base64UrlSafeDecode` | supported | decodes a URL-safe base64 string into bytes |
| `base64UrlSafeEncode` | supported | encodes bytes as a URL-safe base64 string |
| `charsetDecode` | supported | decodes bytes to a string using the named charset |
| `count` | supported | the number of elements in a collection |
| `dateFormat` | supported | formats a date using a strftime-style pattern |
| `daysAfterNow` | supported | days between now and a date value, positive if in the future |
| `daysBeforeNow` | supported | days between a date value and now, positive if in the past |
| `decode` | supported | deprecated alias of charsetDecode |
| `first` | supported | the first element of a collection |
| `format` | supported | deprecated alias of dateFormat |
| `htmlEscape` | supported | escapes HTML special characters in a string |
| `htmlUnescape` | supported | unescapes HTML entities in a string |
| `jsonpath` | supported | evaluates a JSONPath expression against a JSON value |
| `last` | supported | the last element of a collection |
| `location` | supported | the Location header value of an HTTP response value |
| `nth` | supported | the nth element (0-based) of a collection |
| `regex` | supported | the first capture group of a regex matched against a string |
| `replace` | supported | replaces every occurrence of a substring |
| `replaceRegex` | supported | replaces every regex match with a replacement string |
| `split` | supported | splits a string on a separator into a list |
| `toDate` | supported | parses a string into a date using a strftime-style pattern |
| `toFloat` | supported | converts a value to a floating-point number |
| `toHex` | supported | encodes bytes as a lowercase hex string |
| `toInt` | supported | converts a value to an integer |
| `toString` | supported | converts a value to its string representation |
| `urlDecode` | supported | percent-decodes a URL-encoded string |
| `urlEncode` | supported | percent-encodes a string for use in a URL |
| `urlQueryParam` | supported | extracts a named query parameter from a URL string |
| `utf8Decode` | supported | decodes bytes as UTF-8 into a string |
| `utf8Encode` | supported | encodes a string as UTF-8 bytes |
| `xpath` | supported | evaluates an XPath expression against an HTML or XML value |

## Predicates

| Predicate | Status | Description |
|---|---|---|
| `!=` | supported | the value differs from the expected value |
| `<` | supported | the value is less than the expected number or date |
| `<=` | supported | the value is less than or equal to the expected number or date |
| `==` | supported | the value equals the expected value |
| `>` | supported | the value is greater than the expected number or date |
| `>=` | supported | the value is greater than or equal to the expected number or date |
| `contains` | supported | a string, bytes or list value contains the expected element |
| `endsWith` | supported | a string or bytes value ends with the expected suffix |
| `exists` | supported | the query produced a value |
| `includes` | supported | a list contains the expected element (deprecated alias of contains) |
| `isBoolean` | supported | the value is a boolean |
| `isCollection` | supported | the value is a list or object |
| `isDate` | supported | the value is a date |
| `isEmpty` | supported | a string, bytes, list or object value is empty |
| `isFloat` | supported | the value is a floating-point number |
| `isInteger` | supported | the value is an integer |
| `isIpv4` | supported | the value is a valid IPv4 address string |
| `isIpv6` | supported | the value is a valid IPv6 address string |
| `isIsoDate` | supported | the value is a string in ISO 8601 date-time format |
| `isList` | supported | the value is a list |
| `isNumber` | supported | the value is an integer or a float |
| `isObject` | supported | the value is a JSON object |
| `isString` | supported | the value is a string |
| `isUuid` | supported | the value is a string in UUID format |
| `matches` | supported | a string matches the expected regex |
| `startsWith` | supported | a string or bytes value starts with the expected prefix |

## Template functions

| Function | Status | Description |
|---|---|---|
| `newDate` | supported | returns the current date-time |
| `newUuid` | supported | returns a freshly generated UUID v4 |

## Request options

Request `[Options]` section keys.

| Option | Status | Phase | Description |
|---|---|---|---|
| `aws-sigv4` | supported |  | signs the request with AWS Signature Version 4 |
| `cacert` | supported |  | CA certificate bundle used to verify the server (PEM) |
| `cert` | supported |  | client certificate file, optionally with :PASSWORD |
| `compressed` | supported |  | requests a compressed response and decodes it |
| `connect-timeout` | supported |  | maximum time allowed to establish the connection |
| `connect-to` | supported |  | redirects connections for HOST1:PORT1 to HOST2:PORT2 |
| `delay` | supported |  | sleep before sending this entry's request |
| `digest` | supported |  | uses HTTP Digest authentication |
| `fail-with-body` | supported |  | writes the response body of the entry when it fails, before its errors (new in 8.1.0) |
| `header` | supported |  | adds a custom header to the request |
| `http1.0` | supported |  | forces HTTP/1.0 |
| `http1.1` | supported |  | forces HTTP/1.1 |
| `http2` | supported |  | forces HTTP/2 |
| `http2-prior-knowledge` | supported |  | uses HTTP/2 without an HTTP/1.1 upgrade (cleartext HTTP/2 for http://) (new in 8.1.0) |
| `http3` | supported |  | forces HTTP/3 |
| `insecure` | supported |  | skips TLS certificate verification |
| `ipv4` | supported |  | resolves hostnames to IPv4 addresses only |
| `ipv6` | supported |  | resolves hostnames to IPv6 addresses only |
| `key` | supported |  | private key file matching --cert |
| `limit-rate` | supported |  | caps the transfer rate in bytes per second |
| `location` | supported |  | follows HTTP redirects |
| `location-trusted` | supported |  | follows redirects and forwards credentials to every host |
| `max-redirs` | supported |  | maximum number of redirects to follow, -1 for unlimited |
| `max-time` | supported |  | maximum time allowed for the whole transfer |
| `negotiate` | supported |  | uses SPNEGO (Negotiate) authentication |
| `netrc` | supported |  | reads credentials from ~/.netrc, failing if absent |
| `netrc-file` | supported |  | reads credentials from the given netrc-format file |
| `netrc-optional` | supported |  | reads credentials from ~/.netrc if present, else the URL |
| `no-header` | supported |  | removes a header sent to the server, a default one included (repeatable) (new in 8.1.0) |
| `no-jsonpath-coercion` | supported |  | keeps jsonpath results a list of matches (new in 8.1.0) |
| `ntlm` | supported |  | uses NTLM authentication |
| `output` | supported |  | writes the response body to a file instead of stdout |
| `path-as-is` | supported |  | sends the URL path without normalizing /../ or /./ |
| `pinnedpubkey` | supported |  | verifies the server's public key against pinned hashes |
| `proxy` | supported |  | routes the request through the given proxy |
| `repeat` | supported |  | repeats this entry's request N times, -1 for infinite |
| `resolve` | supported |  | provides a custom address for a HOST:PORT pair |
| `retry` | supported |  | maximum retries on entry error, -1 for unlimited |
| `retry-interval` | supported |  | delay between retries |
| `skip` | supported |  | skips this entry without executing it |
| `unix-socket` | supported |  | connects through a Unix domain socket instead of the network |
| `user` | supported |  | adds Basic authentication with USER:PASSWORD |
| `variable` | supported |  | defines a variable local to this entry |
| `verbose` | supported |  | turns on verbose output for this entry |
| `verbosity` | supported |  | sets the verbosity level (brief, verbose, debug) for this entry |
| `very-verbose` | supported |  | turns on very verbose output, including libcurl-style logs |

## Sonde extensions

Valid only in `.sonde` files (a `.hurl` file using one fails to parse). An
entry with a `sonde-stream-*` option reads its body as Server-Sent Events.
See [guides/streaming.md](guides/streaming.md),
[guides/grpc.md](guides/grpc.md),
[decisions/0004-streaming-protocols.md](decisions/0004-streaming-protocols.md) and
[decisions/0005-grpc.md](decisions/0005-grpc.md).

| Construct | Kind | Description |
|---|---|---|
| `[SondeMessages]` | section | makes the entry a WebSocket exchange: ordered send, receive and close steps |
| `[SondeGrpc]` | section | makes the entry a gRPC call of the method its URL path names (`POST http://host/package.Service/Method`), with a JSON request body; empty, the descriptors come from server reflection |
| `proto` | `[SondeGrpc]` key | a `.proto` file compiled at run time (repeatable) |
| `import-path` | `[SondeGrpc]` key | a directory where imports of proto files resolve (repeatable; default: each proto file's directory) |
| `protoset` | `[SondeGrpc]` key | a binary FileDescriptorSet, as written by `protoc --descriptor_set_out --include_imports` or `buf build -o` (repeatable) |
| `send` | `[SondeMessages]` step | sends one message: JSON, a backtick or multiline string or XML as a text frame; base64, hex or file as a binary frame |
| `receive` | `[SondeMessages]` step | waits for the next message, or the next N with `receive: N` |
| `close` | `[SondeMessages]` step | sends a close frame (1000, or `close: CODE`) and waits for the server's |
| `sonde-stream-count` | option | stops an event stream after N events |
| `sonde-stream-max-bytes` | option | stops a stream after N bytes (default 10485760); a WebSocket fails beyond it |
| `sonde-stream-timeout` | option | stops an event stream after this time (default 10s); a WebSocket `receive` still waiting then fails |
| `sondeStream` | query | the data of each event or message received, as a list; `sondeStream "event"`, `"id"`, `"retry"` (SSE) or `"type"` (WebSocket) select another field |
| `sondeGrpc` | query | the status name of a gRPC call (`OK`, `NOT_FOUND`…); `sondeGrpc "code"` is the status code and `sondeGrpc "message"` the status message. A status other than OK fails the entry unless the entry uses this query |

## CLI flags

Occurrence counts are measured across the vendored Hurl 8.0.1 conformance
test scripts (`testdata/conformance/hurl/**/*.sh`).

| Flag | Short | Status | Phase | Hurl test usage | Description |
|---|---|---|---|---|---|
| `--aws-sigv4` |  | supported |  | 4 | signs the request with AWS Signature Version 4 |
| `--cacert` |  | supported |  | 8 | CA certificate bundle used to verify the server (PEM) |
| `--cert` | `-E` | supported |  | 1 | client certificate file, optionally with :PASSWORD |
| `--color` |  | supported |  | 12 | forces colorized output |
| `--compressed` |  | supported |  | 3 | requests a compressed response and decodes it |
| `--connect-timeout` |  | supported |  | 1 | maximum time allowed to establish the connection |
| `--connect-to` |  | supported |  | 3 | redirects connections for HOST1:PORT1 to HOST2:PORT2 |
| `--continue-on-error` |  | supported |  | 18 | keeps running remaining files after a failure |
| `--cookie` | `-b` | supported |  | 1 | reads cookies from a Netscape-format FILE |
| `--cookie-jar` | `-c` | supported |  | 5 | writes cookies to FILE after the run |
| `--curl` |  | supported | 8 | 5 | exports each request as a list of curl commands |
| `--delay` |  | supported |  | 2 | sleep before each request |
| `--digest` |  | supported |  | 1 | uses HTTP Digest authentication |
| `--error-format` |  | supported |  | 3 | controls how error messages are rendered (short or long) |
| `--fail-with-body` |  | supported |  | 5 | writes the response body of a failed entry before its errors (new in 8.1.0) |
| `--file-root` |  | supported |  | 5 | sets the root directory used to resolve file paths |
| `--from-entry` |  | supported |  | 2 | starts execution at the given entry number |
| `--glob` |  | supported | 5 | 6 | adds input files matching the given glob pattern |
| `--header` | `-H` | supported |  | 7 | adds a custom header to every request |
| `--http1.0` | `-0` | supported |  | 1 | forces HTTP/1.0 |
| `--http1.1` |  | supported |  | 1 | forces HTTP/1.1 |
| `--http2` |  | supported |  | 0 | forces HTTP/2 |
| `--http2-prior-knowledge` |  | supported |  | 0 | uses HTTP/2 without an HTTP/1.1 upgrade (cleartext HTTP/2 for http://) (new in 8.1.0) |
| `--http3` |  | supported |  | 1 | forces HTTP/3 |
| `--include` | `-i` | supported |  | 6 | includes the response headers in the output |
| `--insecure` | `-k` | supported |  | 1 | skips TLS certificate verification |
| `--ipv4` | `-4` | supported |  | 7 | resolves hostnames to IPv4 addresses only |
| `--ipv6` | `-6` | supported |  | 1 | resolves hostnames to IPv6 addresses only |
| `--jobs` |  | supported | 5 | 13 | maximum number of parallel jobs, 1 disables parallelism |
| `--json` |  | supported |  | 15 | outputs each file's result as JSON |
| `--key` |  | supported |  | 1 | private key file matching --cert |
| `--limit-rate` |  | supported |  | 1 | caps the transfer rate in bytes per second |
| `--location` | `-L` | supported |  | 8 | follows HTTP redirects |
| `--location-trusted` |  | supported |  | 0 | follows redirects and forwards credentials to every host |
| `--max-filesize` |  | supported |  | 1 | caps the size of a downloaded file |
| `--max-redirs` |  | supported |  | 3 | maximum number of redirects to follow, -1 for unlimited |
| `--max-time` | `-m` | supported |  | 4 | maximum time allowed for the whole transfer |
| `--negotiate` |  | supported |  | 0 | uses SPNEGO (Negotiate) authentication |
| `--netrc` | `-n` | supported |  | 0 | reads credentials from ~/.netrc, failing if absent |
| `--netrc-file` |  | supported |  | 1 | reads credentials from the given netrc-format file |
| `--netrc-optional` |  | supported |  | 0 | reads credentials from ~/.netrc if present, else the URL |
| `--no-assert` |  | supported |  | 1 | ignores asserts defined in the file |
| `--no-color` |  | supported |  | 8 | disables colorized output |
| `--no-cookie-store` |  | supported |  | 2 | disables the cookie store between requests |
| `--no-header` |  | supported |  | 4 | removes a header sent to the server, a default one included (repeatable) (new in 8.1.0) |
| `--no-jsonpath-coercion` |  | supported |  | 1 | keeps jsonpath results a list of matches (new in 8.1.0) |
| `--no-output` |  | supported |  | 18 | suppresses the default last-response-body output |
| `--no-pretty` |  | supported |  | 1 | disables pretty-printing of response output |
| `--no-proxy` |  | supported |  | 0 | lists hosts that bypass the proxy |
| `--ntlm` |  | supported |  | 1 | uses NTLM authentication |
| `--output` | `-o` | supported |  | 14 | writes to FILE instead of stdout |
| `--parallel` |  | supported | 5 | 12 | runs files in parallel (default in test mode) |
| `--path-as-is` |  | supported |  | 1 | sends the URL path without normalizing /../ or /./ |
| `--pinnedpubkey` |  | supported |  | 2 | verifies the server's public key against pinned hashes |
| `--pretty` |  | supported |  | 4 | pretty-prints JSON response output |
| `--progress-bar` |  | supported | 5 | 2 | shows a progress bar in test mode |
| `--proxy` | `-x` | supported |  | 4 | routes the request through the given proxy |
| `--proxy-header` |  | supported |  | 2 | adds a header sent to the proxy only (repeatable) (new in 8.1.0) |
| `--repeat` |  | supported |  | 4 | repeats the input file sequence N times, -1 for infinite |
| `--report-html` |  | supported | 5 | 6 | writes an HTML report to DIR |
| `--report-json` |  | supported | 5 | 5 | writes a JSON report to DIR |
| `--report-junit` |  | supported | 5 | 4 | writes a JUnit XML report to FILE |
| `--report-tap` |  | supported | 5 | 6 | writes a TAP report to FILE |
| `--resolve` |  | supported |  | 3 | provides a custom address for a HOST:PORT pair |
| `--retry` |  | supported |  | 2 | maximum retries on entry error, -1 for unlimited |
| `--retry-interval` |  | supported |  | 2 | delay between retries |
| `--secret` |  | supported |  | 11 | defines a variable whose value is treated as a secret |
| `--secrets-file` |  | supported |  | 1 | defines secrets from a file |
| `--ssl-no-revoke` |  | supported |  | 4 | (Windows) disables certificate revocation checks |
| `--test` |  | supported | 5 | 20 | activates test mode (parallel execution, test-style output) |
| `--to-entry` |  | supported |  | 2 | stops execution at the given entry number |
| `--unix-socket` |  | supported |  | 1 | connects through a Unix domain socket instead of the network |
| `--user` | `-u` | supported |  | 4 | adds Basic authentication with USER:PASSWORD |
| `--user-agent` | `-A` | supported |  | 1 | sets the User-Agent header sent to the server |
| `--variable` |  | supported |  | 16 | defines a variable |
| `--variables-file` |  | supported |  | 2 | defines variables from a properties file |
| `--verbose` | `-v` | supported |  | 43 | turns on verbose output (alias of --verbosity verbose) |
| `--verbosity` |  | supported |  | 2 | sets the verbosity level for debug logging |
| `--very-verbose` |  | supported |  | 6 | turns on very verbose output, including HTTP and libcurl-style logs |

## Environment variables

Occurrence counts are measured across the vendored Hurl 8.0.1 conformance
test scripts (`testdata/conformance/hurl/**/*.sh`).

| Variable | Status | Phase | Hurl test usage | Description |
|---|---|---|---|---|
| `CI` | supported |  | 4 | presence signals a CI environment (affects color/progress defaults) |
| `HURL_COLOR` | supported |  | 5 | same as --color when set truthy, --no-color when falsy |
| `HURL_COMPRESSED` | supported |  | 1 | same as --compressed |
| `HURL_CONNECT_TIMEOUT` | supported |  | 1 | same as --connect-timeout |
| `HURL_CONTINUE_ON_ERROR` | supported |  | 1 | same as --continue-on-error |
| `HURL_DELAY` | supported |  | 2 | same as --delay |
| `HURL_ERROR_FORMAT` | supported |  | 1 | same as --error-format |
| `HURL_FAIL_WITH_BODY` | supported |  | 1 | same as --fail-with-body (new in 8.1.0) |
| `HURL_HEADER` | supported |  | 1 | adds one or more `|`-separated custom headers |
| `HURL_HTTP10` | supported |  | 1 | same as --http1.0 |
| `HURL_HTTP11` | supported |  | 1 | same as --http1.1 |
| `HURL_HTTP2` | supported |  | 0 | same as --http2 |
| `HURL_HTTP2_PRIOR_KNOWLEDGE` | supported |  | 0 | same as --http2-prior-knowledge when truthy, --http1.1 when falsy (new in 8.1.0) |
| `HURL_HTTP3` | supported |  | 0 | same as --http3 |
| `HURL_INSECURE` | supported |  | 1 | same as --insecure |
| `HURL_IPV4` | supported |  | 1 | same as --ipv4 |
| `HURL_IPV6` | supported |  | 1 | same as --ipv6 |
| `HURL_JOBS` | supported | 5 | 1 | same as --jobs |
| `HURL_LIMIT_RATE` | supported |  | 1 | same as --limit-rate |
| `HURL_LOCATION` | supported |  | 1 | same as --location |
| `HURL_LOCATION_TRUSTED` | supported |  | 1 | same as --location-trusted |
| `HURL_MAX_FILESIZE` | supported |  | 1 | same as --max-filesize |
| `HURL_MAX_REDIRS` | supported |  | 2 | same as --max-redirs |
| `HURL_MAX_TIME` | supported |  | 3 | same as --max-time |
| `HURL_NO_ASSERT` | supported |  | 1 | same as --no-assert |
| `HURL_NO_COLOR` | supported |  | 6 | same as --no-color |
| `HURL_NO_COOKIE_STORE` | supported |  | 1 | same as --no-cookie-store |
| `HURL_NO_HEADER` | supported |  | 2 | same as --no-header; several names separated by a vertical bar (new in 8.1.0) |
| `HURL_NO_JSONPATH_COERCION` | supported |  | 1 | same as --no-jsonpath-coercion (new in 8.1.0) |
| `HURL_NO_OUTPUT` | supported |  | 1 | same as --no-output |
| `HURL_NO_PRETTY` | supported |  | 0 | same as --no-pretty |
| `HURL_PARALLEL` | supported |  | 0 | same as --parallel (new in 8.1.0) |
| `HURL_PRETTY` | supported |  | 1 | same as --pretty |
| `HURL_PROGRESS_BAR` | supported |  | 0 | same as --progress-bar when truthy; falsy never shows the bar (new in 8.1.0) |
| `HURL_PROXY_HEADER` | supported |  | 4 | same as --proxy-header; several headers separated by a vertical bar (new in 8.1.0) |
| `HURL_RETRY` | supported |  | 1 | same as --retry |
| `HURL_RETRY_INTERVAL` | supported |  | 1 | same as --retry-interval |
| `HURL_SECRET_name` | supported |  | 3 | defines a secret variable named `name` from its value |
| `HURL_TEST` | supported | 5 | 1 | same as --test |
| `HURL_USER` | supported |  | 1 | same as --user |
| `HURL_USER_AGENT` | supported |  | 1 | same as --user-agent |
| `HURL_VARIABLE_name` | supported |  | 1 | defines a variable named `name` from its value |
| `HURL_VERBOSE` | supported |  | 4 | same as --verbose |
| `HURL_VERBOSITY` | supported |  | 4 | same as --verbosity |
| `HURL_VERY_VERBOSE` | supported |  | 2 | same as --very-verbose |
| `NO_COLOR` | supported |  | 3 | disables colorized output when set to any value |
| `TF_BUILD` | supported |  | 0 | presence signals an Azure Pipelines CI environment |
| `XDG_CONFIG_HOME` | supported |  | 16 | base directory used to locate the config file at $XDG_CONFIG_HOME/hurl/config |

## Config file

Sonde reads `$XDG_CONFIG_HOME/hurl/config` (or `$HOME/.config/hurl/config`
when `XDG_CONFIG_HOME` is not set), a plain-text file of one
`--option[ value]` per line, a value after spaces or `=`. It accepts the
key set of the reference implementation after 8.0.1 (8.0.1 itself reads
four keys and looks in `$HOME/config/hurl/config`). The environment and
flags override the file; `--header` lines add to the other sources, and a
`--secret` name it defines cannot be given again. The CLI and the desktop
app read the same file.

| Key | Status | Phase | Description |
|---|---|---|---|
| `color` | supported |  | forces colorized output |
| `compressed` | supported |  | requests a compressed response and decodes it |
| `connect-timeout` | supported |  | maximum time allowed to establish the connection (seconds when no unit is given) |
| `continue-on-error` | supported |  | keeps running remaining files after a failure |
| `delay` | supported |  | sleep before each request (milliseconds when no unit is given) |
| `error-format` | supported |  | controls how error messages are rendered (short or long) |
| `fail-with-body` | supported |  | writes the response body of a failed entry |
| `header` | supported |  | adds a custom header to every request (repeatable) |
| `http1.0` | supported |  | forces HTTP/1.0 |
| `http1.1` | supported |  | forces HTTP/1.1 |
| `http2` | supported |  | forces HTTP/2 |
| `http3` | supported |  | forces HTTP/3 |
| `insecure` | supported |  | skips TLS certificate verification |
| `ipv6` | supported |  | resolves hostnames to IPv6 addresses only |
| `jobs` | supported |  | maximum number of parallel jobs |
| `limit-rate` | supported |  | caps the transfer rate in bytes per second |
| `location` | supported |  | follows HTTP redirects |
| `location-trusted` | supported |  | follows redirects and forwards credentials to every host |
| `max-filesize` | supported |  | caps the size of a downloaded file |
| `max-redirs` | supported |  | maximum number of redirects to follow, -1 for unlimited |
| `max-time` | supported |  | maximum time allowed for the whole transfer (seconds when no unit is given) |
| `no-assert` | supported |  | ignores asserts defined in the file |
| `no-color` | supported |  | disables colorized output |
| `no-cookie-store` | supported |  | disables the cookie store between requests |
| `no-header` | supported |  | removes a header from every request (repeatable) |
| `no-jsonpath-coercion` | supported |  | keeps JSONPath results uncoerced |
| `no-output` | supported |  | suppresses the default last-response-body output |
| `no-pretty` | supported |  | disables pretty-printing of response output |
| `no-progress-bar` | supported |  | never shows the test progress bar |
| `no-proxy` | supported |  | lists hosts that bypass the proxy |
| `pretty` | supported |  | pretty-prints JSON response output |
| `proxy` | supported |  | routes the request through the given proxy |
| `proxy-header` | supported |  | adds a header sent to the proxy only (repeatable) |
| `retry` | supported |  | maximum retries on entry error, -1 for unlimited |
| `retry-interval` | supported |  | delay between retries (milliseconds when no unit is given) |
| `secret` | supported |  | defines a secret (NAME=VALUE); the name cannot be given again by another source |
| `test` | supported |  | activates test mode |
| `user` | supported |  | adds Basic authentication with USER:PASSWORD |
| `user-agent` | supported |  | sets the User-Agent header sent to the server |
| `variable` | supported |  | defines a variable (NAME=VALUE); the environment and flags override it |
| `verbose` | supported |  | same as --verbose |
| `very-verbose` | supported |  | same as --very-verbose |
| `verbosity` | supported |  | sets the verbosity level (brief, verbose or debug) |

