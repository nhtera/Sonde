<!-- Generated from internal/docs/table.yaml by `make docs`. Do not edit. -->

# Hurl Compatibility

Sonde targets Hurl 8.0.1 files and CLI behavior. This page lists every known
difference. Phase 5 extends it with runtime and CLI gaps.

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
| Regex literal errors other than an invalid `{…}` repetition | Rust `regex` message | Go `regexp` message (same position) |
| Regexes valid in only one engine, e.g. `a{,3}`, `\d{2}{2}`, `a{1001}`, `[]-Z]` | accepted | rejected |
| … and the reverse, e.g. `[0-9[Query]` | rejected | accepted |
| XML body with DTD-declared entities | accepted (libxml2) | rejected: `invalid XML` |
| XML body with text or a comment before the root element | rejected | accepted |
| XML error position | character where libxml2 failed | last character read by Go's `encoding/xml` (same in all test files) |
| JSON nested deeper than 1000 levels | stack overflow | rejected: `nesting is deeper than 1000 levels` |
| File larger than 64 MiB | accepted | rejected: `Issue reading from FILE: file is larger than 64 MiB` |

Accepted as Hurl does: text after a placeholder's variable inside a string,
e.g. `{{a b}}` or `{{a}b}}`, is ignored (and kept by `sonde fmt`); XML
declarations with any encoding or version.

`sonde fmt` is not `hurlfmt`: it only normalizes horizontal whitespace and
line endings (see `docs/architecture.md`, CLI commands) and never reorders
sections. Files already formatted by `hurlfmt` are left unchanged.

## Evaluation (queries, filters, predicates, templates)

Checked against Hurl 8.0.1 by `internal/evaldiff`: 300 asserts over the
fixtures in `testdata/eval/diff` (124 of them failing on purpose, so their
error messages are compared too) give the same result and the same message,
except the rows below (`testdata/eval/diff/known-differences.txt`). Every
JSONPath, XPath, regex and date format literal of the conformance tests is
also accepted or rejected as Hurl does.

| Input | Hurl 8.0.1 | Sonde |
|---|---|---|
| Integers beyond 64 bits in comparisons, e.g. `123456789012345678901234567890 > 9223372036854775807` | compared as digit strings: false | compared numerically: true |
| Rendering of a float whose fraction is negative or below 2.2e-16, e.g. `toString` of `-1.5` or `1e-300` | `-1.5.0`, `0.000…1.0` | `-1.5`, `0.000…1` |
| Negative JSONPath number literal with a fraction, e.g. `$[?@ == -1.5]` | read as `-0.5` | read as `-1.5` |
| NaN (e.g. from XPath `number()` on text) compared with a number | equal to every number | equal to nothing, never less or greater |
| Unprefixed XPath name on an XML document with a default namespace, e.g. `count(//title)` | matches nothing (use `_:title`) | also matches the namespaced elements |
| Malformed XML response body for `xpath`, or XML using entities declared in a DTD | libxml2 recovers what it can, expands the entities | `Invalid XML` error |
| HTML response for `xpath` | libxml2 HTML parser | HTML5 parser: `html`, `head` and `body` always exist |
| `\b` in regexes, and `\W`/`\S` inside a bracket class | Unicode | ASCII |
| Response with several content codings, e.g. `Content-Encoding: gzip, br` | decoded in listed order | decoded in reverse order (last applied first) |
| `Content-Encoding: zstd` | `compression zstd is not supported` | decoded |
| Decoded (decompressed) body larger than 512 MiB | decoded | `Decompression error` |
| JSON response nested deeper than 128 levels, or with a lone surrogate escape (`\ud800`) | `Invalid JSON` | accepted (the surrogate reads as U+FFFD) |
| `urlQueryParam` on a malformed or unusual URL, e.g. `http:///x` | parsed by the Rust `url` crate (WHATWG rules) | parsed by Go's `net/url`: `http:///x` is `empty host`, other messages differ |
| Nested numbers compared by JSONPath `==`, e.g. `$[?@.a == [1]]` on `{"a": [1.0]}` | compared as written: not equal | compared by value: equal |
| Date string with a leap second, e.g. `toDate` of `23:59:60` | keeps the instant at 23:59:59.999999999 | rolls over to the next minute |
| Regex syntax outside the common subset: `(?x)`, `\u{…}`, repetitions over 1000, class set operations `&&` `--`; `\Q…\E` | first group accepted, `\Q…\E` rejected | first group rejected (`Invalid regex`), `\Q…\E` accepted |

Regexes (`matches`, `regex`, `replaceRegex`, JSONPath `match`/`search`) run
on Go's RE2 engine with `\d`, `\w` and `\s` translated to their Unicode
meaning, as in Hurl.

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
| `includes` | supported | a list contains the expected element (alias family of contains) |
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
| `aws-sigv4` | unsupported |  | signs the request with AWS Signature Version 4 (stretch goal; not scheduled (low conformance weight)) |
| `cacert` | planned | 4 | CA certificate bundle used to verify the server (PEM) |
| `cert` | planned | 4 | client certificate file, optionally with :PASSWORD |
| `compressed` | planned | 4 | requests a compressed response and decodes it |
| `connect-timeout` | planned | 4 | maximum time allowed to establish the connection |
| `connect-to` | planned | 4 | redirects connections for HOST1:PORT1 to HOST2:PORT2 |
| `delay` | planned | 4 | sleep before sending this entry's request |
| `digest` | unsupported |  | uses HTTP Digest authentication (HTTP Digest authentication not implemented (low conformance weight)) |
| `header` | planned | 4 | adds a custom header to the request |
| `http1.0` | unsupported |  | forces HTTP/1.0 (Go's net/http cannot send a literal HTTP/1.0 request (Request.Proto is ignored)) |
| `http1.1` | planned | 4 | forces HTTP/1.1 |
| `http2` | planned | 4 | forces HTTP/2 |
| `http3` | planned | 4 | forces HTTP/3 |
| `insecure` | planned | 4 | skips TLS certificate verification |
| `ipv4` | planned | 4 | resolves hostnames to IPv4 addresses only |
| `ipv6` | planned | 4 | resolves hostnames to IPv6 addresses only |
| `key` | planned | 4 | private key file matching --cert |
| `limit-rate` | planned | 4 | caps the transfer rate in bytes per second |
| `location` | planned | 4 | follows HTTP redirects |
| `location-trusted` | planned | 4 | follows redirects and forwards credentials to every host |
| `max-redirs` | planned | 4 | maximum number of redirects to follow, -1 for unlimited |
| `max-time` | planned | 4 | maximum time allowed for the whole transfer |
| `negotiate` | unsupported |  | uses SPNEGO (Negotiate) authentication (SPNEGO authentication not implemented (low conformance weight)) |
| `netrc` | planned | 4 | reads credentials from ~/.netrc, failing if absent |
| `netrc-file` | planned | 4 | reads credentials from the given netrc-format file |
| `netrc-optional` | planned | 4 | reads credentials from ~/.netrc if present, else the URL |
| `ntlm` | unsupported |  | uses NTLM authentication (NTLM authentication not implemented (low conformance weight)) |
| `output` | planned | 4 | writes the response body to a file instead of stdout |
| `path-as-is` | planned | 4 | sends the URL path without normalizing /../ or /./ |
| `pinnedpubkey` | planned | 4 | verifies the server's public key against pinned hashes |
| `proxy` | planned | 4 | routes the request through the given proxy |
| `repeat` | planned | 4 | repeats this entry's request N times, -1 for infinite |
| `resolve` | planned | 4 | provides a custom address for a HOST:PORT pair |
| `retry` | planned | 4 | maximum retries on entry error, -1 for unlimited |
| `retry-interval` | planned | 4 | delay between retries |
| `skip` | planned | 4 | skips this entry without executing it |
| `unix-socket` | planned | 4 | connects through a Unix domain socket instead of the network |
| `user` | planned | 4 | adds Basic authentication with USER:PASSWORD |
| `variable` | planned | 4 | defines a variable local to this entry |
| `verbose` | planned | 4 | turns on verbose output for this entry |
| `verbosity` | planned | 4 | sets the verbosity level (brief, verbose, debug) for this entry |
| `very-verbose` | planned | 4 | turns on very verbose output, including libcurl-style logs |

## CLI flags

Occurrence counts are measured across the vendored Hurl 8.0.1 conformance
test scripts (`testdata/conformance/hurl/**/*.sh`).

| Flag | Short | Status | Phase | Hurl test usage | Description |
|---|---|---|---|---|---|
| `--aws-sigv4` |  | unsupported |  | 4 | signs the request with AWS Signature Version 4 (stretch goal; not scheduled (low conformance weight)) |
| `--cacert` |  | planned | 4 | 8 | CA certificate bundle used to verify the server (PEM) |
| `--cert` | `-E` | planned | 4 | 1 | client certificate file, optionally with :PASSWORD |
| `--color` |  | planned | 4 | 12 | forces colorized output |
| `--compressed` |  | planned | 4 | 3 | requests a compressed response and decodes it |
| `--connect-timeout` |  | planned | 4 | 1 | maximum time allowed to establish the connection |
| `--connect-to` |  | planned | 4 | 3 | redirects connections for HOST1:PORT1 to HOST2:PORT2 |
| `--continue-on-error` |  | planned | 4 | 18 | keeps running remaining files after a failure |
| `--cookie` | `-b` | planned | 4 | 1 | reads cookies from a Netscape-format FILE |
| `--cookie-jar` | `-c` | planned | 4 | 5 | writes cookies to FILE after the run |
| `--curl` |  | planned | 8 | 5 | exports each request as a list of curl commands |
| `--delay` |  | planned | 4 | 2 | sleep before each request |
| `--digest` |  | unsupported |  | 1 | uses HTTP Digest authentication (HTTP Digest authentication not implemented (low conformance weight)) |
| `--error-format` |  | planned | 4 | 3 | controls how error messages are rendered (short or long) |
| `--file-root` |  | planned | 4 | 5 | sets the root directory used to resolve file paths |
| `--from-entry` |  | planned | 4 | 2 | starts execution at the given entry number |
| `--glob` |  | planned | 5 | 6 | adds input files matching the given glob pattern |
| `--header` | `-H` | planned | 4 | 7 | adds a custom header to every request |
| `--http1.0` | `-0` | unsupported |  | 1 | forces HTTP/1.0 (Go's net/http cannot send a literal HTTP/1.0 request (Request.Proto is ignored)) |
| `--http1.1` |  | planned | 4 | 1 | forces HTTP/1.1 |
| `--http2` |  | planned | 4 | 0 | forces HTTP/2 |
| `--http3` |  | planned | 4 | 1 | forces HTTP/3 |
| `--include` | `-i` | planned | 4 | 6 | includes the response headers in the output |
| `--insecure` | `-k` | planned | 4 | 1 | skips TLS certificate verification |
| `--ipv4` | `-4` | planned | 4 | 7 | resolves hostnames to IPv4 addresses only |
| `--ipv6` | `-6` | planned | 4 | 1 | resolves hostnames to IPv6 addresses only |
| `--jobs` |  | planned | 5 | 13 | maximum number of parallel jobs, 1 disables parallelism |
| `--json` |  | planned | 4 | 15 | outputs each file's result as JSON |
| `--key` |  | planned | 4 | 1 | private key file matching --cert |
| `--limit-rate` |  | planned | 4 | 1 | caps the transfer rate in bytes per second |
| `--location` | `-L` | planned | 4 | 8 | follows HTTP redirects |
| `--location-trusted` |  | planned | 4 | 0 | follows redirects and forwards credentials to every host |
| `--max-filesize` |  | planned | 4 | 1 | caps the size of a downloaded file |
| `--max-redirs` |  | planned | 4 | 3 | maximum number of redirects to follow, -1 for unlimited |
| `--max-time` | `-m` | planned | 4 | 4 | maximum time allowed for the whole transfer |
| `--negotiate` |  | unsupported |  | 0 | uses SPNEGO (Negotiate) authentication (SPNEGO authentication not implemented (low conformance weight)) |
| `--netrc` | `-n` | planned | 4 | 0 | reads credentials from ~/.netrc, failing if absent |
| `--netrc-file` |  | planned | 4 | 1 | reads credentials from the given netrc-format file |
| `--netrc-optional` |  | planned | 4 | 0 | reads credentials from ~/.netrc if present, else the URL |
| `--no-assert` |  | planned | 4 | 1 | ignores asserts defined in the file |
| `--no-color` |  | planned | 4 | 8 | disables colorized output |
| `--no-cookie-store` |  | planned | 4 | 2 | disables the cookie store between requests |
| `--no-output` |  | planned | 4 | 18 | suppresses the default last-response-body output |
| `--no-pretty` |  | planned | 4 | 1 | disables pretty-printing of response output |
| `--no-proxy` |  | planned | 4 | 0 | lists hosts that bypass the proxy |
| `--ntlm` |  | unsupported |  | 1 | uses NTLM authentication (NTLM authentication not implemented (low conformance weight)) |
| `--output` | `-o` | planned | 4 | 14 | writes to FILE instead of stdout |
| `--parallel` |  | planned | 5 | 12 | runs files in parallel (default in test mode) |
| `--path-as-is` |  | planned | 4 | 1 | sends the URL path without normalizing /../ or /./ |
| `--pinnedpubkey` |  | planned | 4 | 2 | verifies the server's public key against pinned hashes |
| `--pretty` |  | planned | 4 | 4 | pretty-prints JSON response output |
| `--progress-bar` |  | planned | 5 | 2 | shows a progress bar in test mode |
| `--proxy` | `-x` | planned | 4 | 4 | routes the request through the given proxy |
| `--repeat` |  | planned | 4 | 4 | repeats the input file sequence N times, -1 for infinite |
| `--report-html` |  | planned | 5 | 6 | writes an HTML report to DIR |
| `--report-json` |  | planned | 5 | 5 | writes a JSON report to DIR |
| `--report-junit` |  | planned | 5 | 4 | writes a JUnit XML report to FILE |
| `--report-tap` |  | planned | 5 | 6 | writes a TAP report to FILE |
| `--resolve` |  | planned | 4 | 3 | provides a custom address for a HOST:PORT pair |
| `--retry` |  | planned | 4 | 2 | maximum retries on entry error, -1 for unlimited |
| `--retry-interval` |  | planned | 4 | 2 | delay between retries |
| `--secret` |  | planned | 4 | 11 | defines a variable whose value is treated as a secret |
| `--secrets-file` |  | planned | 4 | 1 | defines secrets from a file |
| `--ssl-no-revoke` |  | unsupported |  | 4 | (Windows) disables certificate revocation checks (Windows-specific certificate revocation control; no equivalent in Go's crypto/tls) |
| `--test` |  | planned | 5 | 20 | activates test mode (parallel execution, test-style output) |
| `--to-entry` |  | planned | 4 | 2 | stops execution at the given entry number |
| `--unix-socket` |  | planned | 4 | 1 | connects through a Unix domain socket instead of the network |
| `--user` | `-u` | planned | 4 | 4 | adds Basic authentication with USER:PASSWORD |
| `--user-agent` | `-A` | planned | 4 | 1 | sets the User-Agent header sent to the server |
| `--variable` |  | planned | 4 | 16 | defines a variable |
| `--variables-file` |  | planned | 4 | 2 | defines variables from a properties file |
| `--verbose` | `-v` | planned | 4 | 43 | turns on verbose output (alias of --verbosity verbose) |
| `--verbosity` |  | planned | 4 | 2 | sets the verbosity level for debug logging |
| `--very-verbose` |  | planned | 4 | 6 | turns on very verbose output, including HTTP and libcurl-style logs |

## Environment variables

Occurrence counts are measured across the vendored Hurl 8.0.1 conformance
test scripts (`testdata/conformance/hurl/**/*.sh`).

| Variable | Status | Phase | Hurl test usage | Description |
|---|---|---|---|---|
| `CI` | planned | 4 | 4 | presence signals a CI environment (affects color/progress defaults) |
| `HURL_COLOR` | planned | 4 | 5 | same as --color when set truthy, --no-color when falsy |
| `HURL_COMPRESSED` | planned | 4 | 1 | same as --compressed |
| `HURL_CONNECT_TIMEOUT` | planned | 4 | 1 | same as --connect-timeout |
| `HURL_CONTINUE_ON_ERROR` | planned | 4 | 1 | same as --continue-on-error |
| `HURL_DELAY` | planned | 4 | 2 | same as --delay |
| `HURL_ERROR_FORMAT` | planned | 4 | 1 | same as --error-format |
| `HURL_HEADER` | planned | 4 | 1 | adds one or more `|`-separated custom headers |
| `HURL_HTTP10` | unsupported |  | 1 | same as --http1.0 (mirrors --http1.0; Go's net/http cannot send a literal HTTP/1.0 request) |
| `HURL_HTTP11` | planned | 4 | 1 | same as --http1.1 |
| `HURL_HTTP2` | planned | 4 | 0 | same as --http2 |
| `HURL_HTTP3` | planned | 4 | 0 | same as --http3 |
| `HURL_INSECURE` | planned | 4 | 1 | same as --insecure |
| `HURL_IPV4` | planned | 4 | 1 | same as --ipv4 |
| `HURL_IPV6` | planned | 4 | 1 | same as --ipv6 |
| `HURL_JOBS` | planned | 5 | 1 | same as --jobs |
| `HURL_LIMIT_RATE` | planned | 4 | 1 | same as --limit-rate |
| `HURL_LOCATION` | planned | 4 | 1 | same as --location |
| `HURL_LOCATION_TRUSTED` | planned | 4 | 1 | same as --location-trusted |
| `HURL_MAX_FILESIZE` | planned | 4 | 1 | same as --max-filesize |
| `HURL_MAX_REDIRS` | planned | 4 | 2 | same as --max-redirs |
| `HURL_MAX_TIME` | planned | 4 | 3 | same as --max-time |
| `HURL_NO_ASSERT` | planned | 4 | 1 | same as --no-assert |
| `HURL_NO_COLOR` | planned | 4 | 6 | same as --no-color |
| `HURL_NO_COOKIE_STORE` | planned | 4 | 1 | same as --no-cookie-store |
| `HURL_NO_OUTPUT` | planned | 4 | 1 | same as --no-output |
| `HURL_NO_PRETTY` | planned | 4 | 0 | same as --no-pretty |
| `HURL_PRETTY` | planned | 4 | 1 | same as --pretty |
| `HURL_RETRY` | planned | 4 | 1 | same as --retry |
| `HURL_RETRY_INTERVAL` | planned | 4 | 1 | same as --retry-interval |
| `HURL_SECRET_name` | planned | 4 | 3 | defines a secret variable named `name` from its value |
| `HURL_TEST` | planned | 5 | 1 | same as --test |
| `HURL_USER` | planned | 4 | 1 | same as --user |
| `HURL_USER_AGENT` | planned | 4 | 1 | same as --user-agent |
| `HURL_VARIABLE_name` | planned | 4 | 1 | defines a variable named `name` from its value |
| `HURL_VERBOSE` | planned | 4 | 4 | same as --verbose |
| `HURL_VERBOSITY` | planned | 4 | 4 | same as --verbosity |
| `HURL_VERY_VERBOSE` | planned | 4 | 2 | same as --very-verbose |
| `NO_COLOR` | planned | 4 | 3 | disables colorized output when set to any value |
| `TF_BUILD` | planned | 4 | 0 | presence signals an Azure Pipelines CI environment |
| `XDG_CONFIG_HOME` | planned | 4 | 16 | base directory used to locate the config file at $XDG_CONFIG_HOME/hurl/config |

## Config file

Hurl 8.0.1 reads `$XDG_CONFIG_HOME/hurl/config` (or `$HOME/config/hurl/config`
as a fallback), a plain-text file of one `--option[ value]` per line.

| Key | Status | Phase | Description |
|---|---|---|---|
| `header` | planned | 4 | adds a custom header to every request |
| `max-redirs` | planned | 4 | maximum number of redirects to follow, -1 for unlimited |
| `user-agent` | planned | 4 | sets the User-Agent header sent to the server |
| `verbose` | planned | 4 | same as --verbose |

