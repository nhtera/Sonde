# `sonde.yaml`

Owner: this file (`internal/config`). `sonde.yaml` is the only
project file Sonde reads on its own; it selects environments for a test run
and nothing else. It never widens file access and never changes what a
request file can read or write — that stays a CLI-only concern
(`--file-root`).

`sonde.yaml` sits inside the repository being tested, so it is **untrusted
input** (see `docs/architecture.md` §9): a CLI flag always overrides
anything it sets.

## Schema

```yaml
version: 1

environments:
  local:
    variables:
      base_url: http://localhost:8080
      retries: 3
      strict: true
      extra: null
    variables_files:
      - env/local.vars
    secrets_files:
      - env/local.secrets

  staging:
    variables:
      base_url: https://staging.example.internal

defaults:
  env: local
  jobs: 4

openapi:
  spec: openapi.yaml
  server: http://localhost:3000/v1
  strict: false
  exclude_operations: ["GET /health"]
  exclude_files: ["legacy/**/*.hurl"]
```

- `version` (required): must be `1`. Any other value, or a missing key, is
  an error.
- `environments` (optional): a map from environment name to:
  - `variables` (optional): a map of name to value. A value is a plain YAML
    scalar — string, boolean, number or `null` — typed the same way a
    `--variable` or variables-file entry is (`internal/config/value.go`).
    Lists and nested maps are rejected.
  - `variables_files` (optional): paths to variables files, in the existing
    `name=value` format (`internal/config/properties.go`), applied in order
    after `variables` (a name in a later file wins).
  - `secrets_files` (optional): paths to secrets files, same format,
    loaded as secrets (redacted), same rules as `--secrets-file`: a name
    already defined by an earlier secrets source is an error.
- `defaults` (optional):
  - `env`: the environment used when neither `--env` nor `SONDE_ENV` is set.
  - `jobs`: the default `--jobs` value when the flag is not given; an
    integer from `0` (unset) to `64`. An untrusted `sonde.yaml` cannot
    size a run's parallelism (and so its goroutines, HTTP clients and file
    descriptors) beyond that cap.
- `openapi` (optional): validates every response of the project's files
  against an OpenAPI contract (see [guides/openapi.md](./guides/openapi.md)).
  - `spec` (required): the OpenAPI file, a relative path inside the
    `sonde.yaml` directory. A remote (`http(s)://`) spec cannot be set
    here: remote fetching is enabled only on the command line
    (`--openapi URL --openapi-allow-remote`).
  - `server`: a base URL replacing the spec's `servers` for matching
    request URLs, like `--openapi-server`.
  - `strict`: a request no operation matches fails (exit 4), like
    `--openapi-strict`; by default it is a warning.
  - `exclude_operations`: operations never validated, `METHOD /path` with
    the path as written in the spec (`GET /pets/{petId}`). A method the spec
    does not document on a documented path can be excluded too
    (`DELETE /pets/{petId}`), which silences its unmatched warning (or
    strict failure).
  - `exclude_files`: globs of request files never validated, relative to
    the `sonde.yaml` directory; `**` matches any number of directories.

  Command line flags win over these keys one by one (`--openapi` replaces
  `spec`); the exclusions still apply.

Any other top-level or nested key is an error naming the offending line.
There is no `file_root` key and no way to set report options or inline
secret values — those stay CLI-only. A `sonde.yaml` file is exactly one
YAML document (a second `---`-separated document is an error) and at most
1 MiB; the same 1 MiB cap and "must be a regular file" rule apply to every
`variables_files`/`secrets_files` entry it references (a FIFO or other
special file is rejected rather than read, since reading one with no
writer would hang the run).

## Precedence

`sonde.yaml` variables and secrets sit at the **lowest** precedence, below
every CLI and environment-variable source (`docs/architecture.md` §5,
"Variable precedence"):

1. `sonde.yaml` environment: `variables`, then `variables_files`
2. `HURL_VARIABLE_*` / `SONDE_VARIABLE_*` env vars
3. `--variables-file`
4. data row (`--data`)
5. `--variable`
6. entry `[Options] variable:`
7. captures during the run

Secrets follow the same shape: `sonde.yaml` `secrets_files` is the lowest
secret source, below `HURL_SECRET_*`/`SONDE_SECRET_*`, `--secrets-file`,
`--secret`, `--data-secret` columns and `redact` captures. A name defined
twice **inside** `sonde.yaml` itself (the same name in two `secrets_files`
entries) is still an error, exactly like a duplicate anywhere else. Once
resolved, though, a `sonde.yaml` secret is a plain lowest-precedence
default: the same name from any of those higher sources silently overrides
it, with no error — it is only a duplicate among `HURL_SECRET_*`/
`SONDE_SECRET_*`, `--secrets-file` and `--secret` themselves (which have no
config-file role to sit below) that is still rejected.

Environment selection, highest precedence first:

1. `--env NAME`
2. `SONDE_ENV` environment variable
3. `defaults.env`

An `--env`/`SONDE_ENV`/`defaults.env` value that names an environment not
present in `sonde.yaml` is an error listing the environments that do exist;
the CLI exits with status 1. No environment selected (none of the three set,
and `environments` is absent or unused) is not an error: a run simply has no
`sonde.yaml` variables or secrets.

The same distinction applies when a file has no `sonde.yaml` above it at
all (and no `--config` override): an explicit `--env NAME` is then an error
(status 1, naming the file and the environment) — there is no project file
for it to select an environment from. `SONDE_ENV` alone, with no `--env`,
is silently ignored in that case instead: it only ever selects an
environment where a `sonde.yaml` actually exists to apply it to.

## Discovery

For each input file, Sonde looks for `sonde.yaml` in that file's directory,
then its parent, and so on, stopping at the first one found. Different
input files in the same run may resolve to different `sonde.yaml` files
this way; each directory's answer is cached for the run so the walk
happens once per directory.

The walk is bounded, not open-ended up to the filesystem root, so a file
placed somewhere Sonde does not control cannot configure a run it was
never meant to:

- It stops at the first directory containing a `.git` entry (a directory
  for a normal checkout, a file for a worktree or submodule) — that
  directory's own `sonde.yaml` is still eligible, but nothing above the
  repository being tested is considered. A run outside any repository has
  no such boundary short of the filesystem root.
- On Unix, a candidate `sonde.yaml` not owned by the user running Sonde,
  or writable by group or others, is treated as absent (with a recorded
  warning) rather than used: it cannot be trusted to be the project's own
  file even if it happens to be found. This check does not exist on
  Windows, whose permission model does not express it the same way.

Neither restriction applies to `--config FILE` (a trusted, CLI-given
path): it overrides discovery entirely, is used for every input, and does
not have to be named `sonde.yaml`.

When input files resolve to different `sonde.yaml` files this way, each
file's own `variables`/`secrets` still apply only to that file's job. The
one setting that isn't per-file is `defaults.jobs`, since `--jobs` picks a
single worker count for the whole run: Sonde uses whichever project's
`defaults.jobs` it discovers first, in input-file order, and does not
merge or compare it against any other project's value. Give every
`sonde.yaml` in a multi-project run the same `defaults.jobs` (or none at
all, relying on `--jobs`) to avoid depending on that order.

## File access

Every path in `variables_files` and `secrets_files` is resolved relative to
the directory containing that `sonde.yaml` file, through a sandbox rooted
there (`internal/sandbox`). A path that is absolute, that escapes that
directory with `..`, or that reaches outside it through a symbolic link is
rejected. `sonde.yaml` cannot reference anything outside its own directory,
and it has no key to change that (no `file_root`). Each referenced file
must also be a regular file no larger than 1 MiB (see Schema); a FIFO or a
directory is rejected before it is ever opened for reading, and the size
check happens before the content is used.
