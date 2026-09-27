# Security

This page explains what Sonde treats as trusted, what it does to limit the
damage untrusted input can do, and how to report a vulnerability. It is the
user-facing companion to [architecture.md](architecture.md) §9 (Security
Model), which is the developer-facing version of the same table.

To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Trust model

- **Trusted:** the command line you type, flags, environment variables you
  set yourself, and files under your user config directory.
- **Untrusted:** a request file (`.hurl`) and a `sonde.yaml` you did not
  write yourself — for example one checked out from a repository someone
  else controls, or a collection someone sent you. Sonde is designed so
  that running an untrusted request file or `sonde.yaml` cannot read or
  write files outside the project, cannot change what files a run is
  allowed to touch, and cannot leak a secret you supplied on the trusted
  command line.

## Secrets never appear in the clear

Every secret Sonde ever holds — a `--secret` value, a `SONDE_SECRET_*`
environment variable, a `--data-secret` data-file column, a `sonde.yaml`
`secrets_files` entry, or a value captured at runtime with `redact` — is
masked out of everything Sonde writes: the terminal (including `-v` and
`-vv`), `--json`, every report format (`--report-junit`, `--report-tap`,
`--report-json`, `--report-html`), the `--curl` command file, `sonde export
curl`, and the `--cookie-jar` file. Masking also catches the value's
base64, URL-escaped and JSON-escaped forms, so a secret that a server
happens to echo back re-encoded still comes out as `***`.

A short secret (under 4 characters) is still masked, but Sonde warns that a
short value is prone to matching text it was never meant to redact — pick a
longer one where you can.

## Local files are sandboxed

An option that names a file *inside a request file* — `file,` request
bodies and multipart parts, `output`, `--cacert`/`--cert`/`--key`,
`--netrc-file`, `--unix-socket` — is confined to a file root (the current
directory, or wherever `--file-root` points): `..`, an absolute path, or a
symbolic link that would leave the root are all rejected. The same option
given directly on the command line is trusted and not sandboxed, since you
typed it yourself.

`sonde.yaml` is untrusted input with its own, stricter rule: a
`variables_files` or `secrets_files` entry must be a relative path that
stays inside the directory holding that `sonde.yaml` — even an absolute
path is rejected outright, not just one that escapes via `..` or a
symlink. A `sonde.yaml` can never widen what a run is allowed to touch; it
can only be more specific about variables and secrets within its own
directory. Sonde also refuses to use a candidate `sonde.yaml` that is
writable by anyone other than its owner.

## No ambient credentials on the wrong host

A `~/.netrc` credential is only ever sent to the exact host, port and
scheme it was written for. When a redirect or `--resolve`/`--connect-to`
sends a request somewhere else, that credential does not follow it.

## Templates cannot read the environment

There is no `getEnv` function in a request file's templates (Hurl 8 does
not have one either). A future built-in that needs process environment
access will be opt-in, allowlisted, and treated as a secret automatically.

## Reports and imports

The HTML report (`--report-html`) renders every value — URLs, headers,
bodies, filenames, assert messages, captures — through Go's `html/template`
autoescaper, sends a strict Content-Security-Policy
(`default-src 'none'; style-src 'unsafe-inline'`), and loads nothing from
the network: no remote fonts, scripts, or images.

Every importer (`sonde import curl|postman|opencollection|http`, `sonde
import openapi`) is exercised by a fuzz target, runs no code found in its
input (an imported script becomes a comment, never something Sonde
executes), and caps output size. `--openapi` never fetches a remote `$ref`
unless you pass `--openapi-allow-remote`.

A decoded response body is capped (512 MiB by default) so a decompression
bomb cannot exhaust memory.

## The LSP (`sonde lsp`)

The language server treats `sonde.yaml`, `variables_files` and
`secrets_files` exactly like a run does: as untrusted input, read only to
extract variable *names* for completion and hover. A secrets_files value
is parsed only far enough to detect a name clash; the value itself is
never kept, never shown in a hover, and never sent anywhere. The LSP never
makes an HTTP request and never reads a file outside the workspace folders
the client gave it.

## Supply chain

Dependencies are kept minimal and reviewed with `go mod why`; `govulncheck`
and `gosec` (via `golangci-lint`) run in CI on every push, plus a nightly
fuzz run (`.github/workflows/nightly-fuzz.yml`) across every parser and
importer. CI workflows request only `contents: read`, and third-party
GitHub Actions are pinned by commit SHA. Releases are signed (`cosign`,
keyless via GitHub OIDC) and ship a Software Bill of Materials (`syft`);
publish credentials live only in the protected `release` environment.

## Reporting a vulnerability

See [SECURITY.md](../SECURITY.md): please report privately through GitHub
Security Advisories rather than a public issue.
