# 0006: MCP server

Status: Accepted. Date: 2026-09-28.

## Context

Phase 14 adds `sonde mcp`, a Model Context Protocol server for AI agents
(Claude Code, Cursor, VS Code). With it an agent can find request files,
see what they send, and, when the user allows it, run them and read the
results.

The agent calling the tools is not the user. It acts on whatever it has
read: a README, an issue, a response body. So the question is what a
process started by the user may do on the user's machine and network, when
an LLM that reads untrusted text sends its commands. The requirements that
follow:

- read-only by default;
- every capability granted on the command line, at start;
- no path outside the root;
- no connection to a host the user did not name;
- no secret value in a tool result or in the log;
- results an agent can act on.

## Decision

### Library

The official Go SDK, `github.com/modelcontextprotocol/go-sdk` v1.8.0. Its
source files are under an MIT license (the repository's LICENSE file also
carries Apache-2.0 text, which GitHub reports as NOASSERTION). Either
license is on the allowed list. The modules it adds to the binary are MIT
or BSD-3-Clause: `google/jsonschema-go`, `segmentio/encoding` and
`segmentio/asm`, `yosida95/uritemplate`, `golang.org/x/oauth2` and
`golang.org/x/sys`. They are covered by `govulncheck` and the SBOM like
every other dependency. The SDK does not recover from a panic in a tool
handler, so `internal/mcp` adds a middleware that turns one into an error
for that request.

A hand-written server was the fallback the plan kept for a license problem,
and none exists. It would also not reuse the language server's framing:
MCP over stdio is newline-delimited JSON, not `Content-Length` frames. The
SDK negotiates every protocol version from 2024-11-05 to 2026-07-28, and
validates tool input against the schemas it infers from Go types.

### Packaging

Measured on 2026-09-28 (darwin/arm64, stripped, `sonde --version`, median
of 300 runs):

| | Binary | Cold start |
|---|---|---|
| v1.2.0 | 20.97 MB | 8.86–8.90 ms |
| SDK probe (one tool) | 22.84 MB | 9.32 ms (+5 %) |
| `sonde mcp` as shipped | 22.97 MB (+2.0 MB) | 8.60 ms (within noise) |

Both stay within the rule of earlier phases (under 8 MB and 20 %), so
Sonde ships one binary. A separate one would split the Homebrew and Scoop
packages, allow version skew, and break the `"command": "sonde"` client
configurations.

### Tools

| Tool | When | Output |
|---|---|---|
| `sonde_list` | always | request files under the root or a directory of it; environments of each `sonde.yaml` |
| `sonde_check` | always | per file: first syntax error, or entry summary (method, URL as written, sections, option names) |
| `sonde_run` | `--allow-run` | `{result, failures, truncated}`: the `--json` result of the file, plus each failing entry's last response body |

`sonde_check` and `sonde_run` read only `.hurl` and `.sonde` files: a parse
error quotes its source, so no other file is ever parsed. The result reuses
the documented v1 JSON schema ([report-json.md](../report-json.md)). The
failing bodies sit beside it (up to 10, 16 KiB each, 256 KiB in all)
because the schema holds a body only as a stored file's path.

### Run capability

- **Off by default.** `sonde_run` is not registered without `--allow-run`,
  so an agent cannot even see it.
- **An allowlist, with no default.** `--allow-run` without `--allow-host`
  is a usage error. "Localhost" would be the most dangerous default: local
  admin ports, databases and container APIs live there. A silent deny-all
  would register a tool that always fails. `*` allows everything,
  explicitly, with a warning.
- **Patterns.** Exact name, `name:port`, `*.domain` (not the apex), an
  address, a CIDR range. Names are never resolved and never aliased
  (`localhost` is not `127.0.0.1`). A name matches only if it is plain
  ASCII (letters, digits, `-`, `_`), and a zone only on an IPv6 address.
  The system resolver reads other forms differently: `a.com%.evil.example`
  would be looked up whole, and a Unicode name is dialed as punycode.
  Short or hexadecimal IPv4 forms (`127.1`, `0x7f000001`), which some
  resolvers read as addresses, never match either.
- **Two checks, one matcher.** `internal/netpolicy` normalizes hosts and
  matches patterns. `internal/httpx` calls it for:
  - the URL of every request, redirect hop and WebSocket handshake (gRPC
    calls are requests);
  - every connection it dials, after `connect-to`, each `resolve` address,
    the proxy of an `HTTP_PROXY`, and the SOCKS proxy hop.

  Neither check alone is enough. A proxy hides the target from the dial,
  and `connect-to` or `resolve` hide it from the URL. Using the same code for
  both is what keeps them from disagreeing.
- **Refused options.** `proxy`, `connect-to`, `resolve`, `unix-socket`,
  `netrc`, `netrc-file`, `netrc-optional` and `output` fail the entry before
  anything is sent. The first four move the connection away from the URL's
  host. The `netrc` options send stored credentials. `output` writes files
  (`-` would write into the protocol stream).
- **Secrets from trusted sources only.** Secrets come from `sonde.yaml`
  `secrets_files`, `SONDE_SECRET_*`, `--secret` and `--secrets-file`. None
  come from a tool call: an agent-chosen "secret" such as `e` would only
  blind the results and the audit log. Call variables are bounded (64
  names, 4 KiB values, scalars only) and cannot redefine a secret.
- **The root.** `--root` (default: the working directory, symbolic links
  resolved) bounds every tool path and is each run's file root. The
  server changes into it, so relative `cacert`, `cert` and `key` paths,
  which are read from the working directory, stay under it. A `sonde.yaml`
  found above the root is ignored, and one that is a symbolic link is
  refused. The MCP `roots` a client
  announces are not used for authorization.
- **Bounds.** One run at a time. Each run ends at `--run-timeout` (60 s by
  default, from 1 s to 10 minutes), and a call the client cancels stops
  its run. A response body is capped at 16 MiB. `sonde_list` stops at 2000
  request files and 100 `sonde.yaml` files.
- **Audit.** One redacted line per tool call on standard error. Standard
  output carries only protocol messages.

### Public API

The policy reaches the engine through `internal/enginex.SetHosts`, not
through a new `engine.Options` field. Its shape spans the transport and
option refusal and has not been proven. A public field would freeze it into
the v1 API. It can be promoted, additively, if the desktop application or
`go test` embedding needs it. `engine` and `exchange` are unchanged.

## Consequences

- An agent can run the project's request files against the hosts the user
  lists, and gets structured, redacted results.
- A file that uses a refused option runs under `sonde --test` but fails
  under `sonde mcp`, with an error naming the option.
- The allowlist expresses trust in names. A broad wildcard trusts every
  subdomain, including one an attacker controls. A secret the request file
  sends to an allowed host goes there, as it would in a manual run.
- Short or transformed secrets (under 4 characters, or HMAC-derived) can
  leak into results as they can into reports, but the reader is now an LLM
  that may repeat them.
- New tools and new output fields may appear in minor releases. A tool
  keeps its name, arguments and meaning within v1.x
  ([stability.md](../stability.md)).
