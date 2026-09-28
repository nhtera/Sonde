# AI agents (`sonde mcp`)

`sonde mcp` lets an AI agent (Claude Code, Cursor, VS Code, or any other
[Model Context Protocol](https://modelcontextprotocol.io) client) find,
check and run your request files. It is an MCP server on standard input and
output: the client starts it and talks to it; you never run it by hand.

By default it is read-only. The agent can list request files and see what
they send, but cannot send a request. Running files is a capability you
grant when the server starts, together with the list of hosts runs may
contact. The design is recorded in
[decisions/0006-mcp-server.md](../decisions/0006-mcp-server.md).

## Setup

**Claude Code**, from the project directory:

```sh
claude mcp add sonde -- sonde mcp
```

With `--scope project`, the server is written to `.mcp.json` and shared with
everyone working on the repository:

```json
{
  "mcpServers": {
    "sonde": { "type": "stdio", "command": "sonde", "args": ["mcp"] }
  }
}
```

**Cursor**, in `.cursor/mcp.json` (or `~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "sonde": { "command": "sonde", "args": ["mcp"] }
  }
}
```

**VS Code**, in `.vscode/mcp.json` (the key is `servers`):

```json
{
  "servers": {
    "sonde": { "type": "stdio", "command": "sonde", "args": ["mcp"] }
  }
}
```

The server sees the directory it starts in (clients start it in the
project), or the one `--root` names, and works from there: relative
`cacert`, `cert` and `key` paths in request files are read from the root.

## Tools

| Tool | Available | What it does |
|---|---|---|
| `sonde_list` | always | Lists the `.hurl` and `.sonde` files under the root (or one of its directories), and the environments of each `sonde.yaml` |
| `sonde_check` | always | Parses up to 100 files; returns each one's first syntax error, or its entries: method, URL as written, sections and option names |
| `sonde_run` | with `--allow-run` | Runs one file and returns its result |

`sonde_list` skips hidden directories and symbolic links, and stops at 2000
files. `sonde_check` and `sonde_run` read only `.hurl` and `.sonde` files.

### `sonde_run`

| Argument | Meaning |
|---|---|
| `path` | The request file, relative to the root |
| `env` | A `sonde.yaml` environment. Default: `--env`, then `SONDE_ENV`, then the `sonde.yaml`'s `defaults.env` |
| `variables` | Variables for this run: up to 64 names, each a string (up to 4 KiB), a number or a boolean. They win over `--variable` and the environment's variables, and cannot redefine a secret |

The result has:

- `success`, `env`, and `error` for a parse error or a run that stopped early;
- `result`: the file's result exactly as `sonde --json` prints it
  ([report-json.md](../report-json.md)), with the file named relative to the
  root;
- `failures`: each failing entry (up to 10) with its errors, the status and
  content type of its last response, and that response's body (up to 16 KiB,
  left out when it is not text).

The whole output stays under 256 KiB: response bodies are dropped first,
then `result`, and `truncated` says so. A run with a file that fails is a
tool error (`isError`), so the agent sees it failed.

Each run starts fresh: no cookies carry over from one call to the next.
Runs happen one at a time, each within `--run-timeout` (default 60 s,
from 1 s to 10 minutes). When the client cancels a call, its run stops.
The timeout also ends an entry that keeps retrying (`retry: -1`) against a
host that is not allowed.

## Allowing runs

```sh
sonde mcp --allow-run --allow-host localhost:8080 --allow-host '*.staging.example.com'
```

`--allow-run` without `--allow-host` is an error: there is no default list.

| Pattern | Matches |
|---|---|
| `api.example.com` | That host, any port |
| `api.example.com:8443` | That host and port |
| `*.example.com` | Any subdomain of `example.com`, not `example.com` itself |
| `10.1.2.3`, `[::1]:8080` | That address (port optional) |
| `10.0.0.0/8`, `fd00::/8` | Addresses in that range |
| `*` | Every host (a warning is printed) |

Names are case-insensitive and never resolved: `localhost` does not match
`127.0.0.1`, and an address pattern only matches URLs written with an
address. List every form your files use. Only plain ASCII names match:
write an internationalized name in its punycode form (`xn--…`), in the
file and in the pattern. A host with anything else in it, such as a `%`,
is refused.

The list is checked twice. Every request URL and every redirect it follows
must name an allowed host, and every connection Sonde opens must go to one.
The second check covers proxies. A proxy set with `HTTP_PROXY` or
`HTTPS_PROXY` must be allowed too. It also covers entries that would
connect elsewhere than their URL says.

For the same reason, an entry using one of these options fails, without
sending anything, under `sonde mcp`:

| Option | Why |
|---|---|
| `proxy`, `connect-to`, `resolve`, `unix-socket` | They send the connection somewhere else than the URL's host |
| `netrc`, `netrc-file`, `netrc-optional` | They send stored credentials |
| `output` | It writes a file, or would corrupt the protocol on standard output |

Such files still run with `sonde --test`.

## Variables and secrets

Runs get variables and secrets the way a command-line run does:

- from `sonde.yaml` environments ([sonde-yaml.md](../sonde-yaml.md));
- from the server's `--variable`, `--variables-file`, `--secret` and
  `--secrets-file` flags;
- from `SONDE_VARIABLE_*` and `SONDE_SECRET_*` in the server's environment.

An MCP client can set the server's environment. For example, in `.mcp.json`:

```json
"sonde": {
  "type": "stdio",
  "command": "sonde",
  "args": ["mcp", "--allow-run", "--allow-host", "api.staging.example.com"],
  "env": { "SONDE_SECRET_token": "${STAGING_TOKEN}" }
}
```

Only a `sonde.yaml` under the root is used: one in a parent directory is
ignored, and one that is a symbolic link is refused. Secrets are redacted from everything a tool returns and from the
audit log, as they are from reports. An agent cannot add secrets: a secret
chosen by the agent would only hide text from it.

## Security

The server treats the agent as untrusted. The agent may be following
instructions it read in a file, a web page, or a response body.

- **Nothing unless you start the server with it.** Running files, and the
  hosts runs may contact, are set on the command line. No tool call can
  widen them.
- **The root.** Tools read only files under `--root`. A path leaving it, directly or
  through a symbolic link, is refused. So are request-file paths such as
  `file,` bodies, certificates and `.proto` files, which a normal run also
  confines to its file root.
- **The allowlist is a statement of trust.** A run can send a secret to any
  allowed host, as the request file says. Allow the hosts you would run the
  files against yourself, and prefer exact names to broad wildcards.
- **Response bodies are data.** A server under test can return text written
  to steer an agent. The tool result says so, but only the agent can act on
  it.
- **The audit log.** The server writes one line per tool call to standard
  error: the tool, the file, the environment, the outcome and the duration.
  Most clients keep it in their MCP logs.

See [security.md](../security.md) for Sonde's security model as a whole.
