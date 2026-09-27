# OpenAPI contracts

Sonde checks responses against an OpenAPI description without touching
your request files: `--openapi` validates every response of a run, and
`sonde import openapi` writes a request file per operation to start from.
OpenAPI 3.0 and 3.1 are supported; a Swagger 2.0 document is converted on
load.

## Design-first workflow

```sh
sonde import openapi openapi.yaml -o api      # one file per operation + api/sonde.yaml
$EDITOR api/pets/get-pet.hurl                 # add captures, asserts, real values
sonde test --openapi openapi.yaml api         # every response checked against the spec
```

No backend yet? `sonde mock openapi.yaml` serves the spec's examples to
run against: see [mock-server.md](mock-server.md).

## Validating responses

```sh
sonde --test --openapi openapi.yaml tests/
```

After an entry's explicit asserts, its final response (after redirects)
is matched to an operation and checked for:

- a documented status: the exact code, its range (`2XX`) or `default`;
- the required response headers, and their schemas;
- a documented content type (`application/json; charset=utf-8` matches
  `application/json`, and `image/*` or `*/*` match any image or type);
- a JSON body (`application/json`, `*/*+json`, `*/json`) against its
  schema. Other bodies are checked for their content type only.

A `HEAD` request without its own operation is checked against the `GET`
operation, without a body. `301`, `304`, `307` and `308` responses are
not checked.

A violation fails the entry like an assert (exit code 4). It is reported
at the entry's `HTTP` line, or at its request line without one:

```
error: Contract violation
  --> tests/get-pet.hurl:2:1
   |
   | GET {{base_url}}/pets/2
 2 | HTTP 200
   | ^^^^^^^^ value must be an integer (type)
   |          at: /id
   |          spec: #/paths/~1pets~1{petId}/get/responses/200/content/application~1json/schema
   |
```

`at` locates the problem in the body (a JSON pointer) or names the header.
`spec` points into the spec: at the response, its header, or its body
schema. JSON results and reports carry the same data under
`sonde.contract` (see [report-json.md](../report-json.md)). Messages are
redacted like every other output.

`--no-assert` skips contract checks too. The check honors `--retry`, and a
failing contract check fails the attempt. An entry whose status or
version assert fails, or whose captures fail, stops there: its response is
not checked against the contract.

### Matching requests to operations

- The request path loses the spec's server base path: for a server
  `https://api.example.com/v1`, the request `http://localhost:8080/v1/pets/7`
  matches `/pets/{petId}`. The host does not matter; server variables take
  their defaults; with several servers, the longest matching base path
  wins.
- `--openapi-server URL` replaces the spec's servers, for a local server
  mounted elsewhere: `--openapi-server http://localhost:8080` matches
  `http://localhost:8080/pets/7`.
- Literal segments win over templates: `/pets/mine` matches `/pets/mine`,
  not `/pets/{petId}`, unless only `/pets/{petId}` documents the method.
- A request no operation matches, or whose method the path does not
  document, is a warning. With `--openapi-strict` it is a violation.

### Flags

| Flag | Meaning |
|---|---|
| `--openapi SPEC` | The spec: a file, or an `http(s)` URL with `--openapi-allow-remote`. |
| `--openapi-server URL` | Base URL replacing the spec's servers when matching. |
| `--openapi-strict` | A request no operation matches fails. |
| `--openapi-allow-remote` | Allows a remote spec and remote `$ref` targets. |

A spec that fails to load stops the run before any request (exit code 1).

### In `sonde.yaml`

```yaml
openapi:
  spec: openapi.yaml             # inside the sonde.yaml directory
  server: http://localhost:8080  # optional, like --openapi-server
  strict: false                  # optional, like --openapi-strict
  exclude_operations: ["GET /health"]
  exclude_files: ["legacy/**/*.hurl"]
```

Every file of the project is then validated without a flag. Command line
flags win key by key (`--openapi` replaces `spec`), and the exclusions
still apply. See [sonde-yaml.md](../sonde-yaml.md).

### Trust

A spec is untrusted input.

- A `$ref` may name another file only inside the directory of the spec.
- A `$ref` target must be a regular file.
- Remote specs and remote `$ref`s are fetched only with
  `--openapi-allow-remote`, which `sonde.yaml` cannot set. A remote fetch
  does not use the run's `--proxy`, `--cacert` or `--insecure` options.
  It follows redirects, and each read times out after 30 seconds.
- Each document is limited to 64 MiB.
- Schemas never make Sonde read files: `$schema` and `$dynamicRef` are not
  resolved.

### Limits

- Response bodies are validated as JSON only.
- These JSON Schema keywords of OpenAPI 3.1 are read but not enforced:
  - `if`/`then`/`else`;
  - `contains`;
  - `dependentRequired` and `dependentSchemas`;
  - `patternProperties` and `propertyNames`;
  - `unevaluatedProperties` and `unevaluatedItems`;
  - `$dynamicRef`.

  Everything else is enforced, including type lists (`[string, "null"]`),
  `const`, numeric `exclusiveMinimum`/`exclusiveMaximum`, `prefixItems`
  and `not`.
- Only the first failing documented header is reported.
- Path-level and operation-level `servers` are not used for matching.
  Use `--openapi-server` when they differ from the top-level ones.
- A spec is loaded once per run, then shared by every file and data row.
  A 10 MB spec (about 1,200 operations) loads in about one second.

## Importing a spec

```sh
sonde import openapi openapi.yaml -o api [--group tag|path|flat] [--base-url-var base_url]
```

Each operation becomes one file. The common flags (`--ext`, `--force`,
`--dry-run`) and the writer's rules are in
[import-export.md](import-export.md). Each file contains:

- **Comments:** the operationId and summary.
- **URL:** `{{base_url}}` followed by the path. Path parameters become
  variables, such as `{{petId}}`.
- **Required parameters:** query, header and cookie parameters, with their
  example values.
- **Security:** placeholders for the operation's first security
  requirement:

  | Scheme | Written as |
  |---|---|
  | bearer, OAuth 2, OpenID Connect | `Authorization: Bearer {{token}}` |
  | basic | `[BasicAuth]` with `{{username}}` and `{{password}}` |
  | API key | a header, query parameter or cookie named by the scheme, holding a variable |

  The summary says which variables to set, typically as secrets
  (`--secret token=...`).
- **Body:** the first supported media type, in this order: JSON (with a
  `Content-Type` header for `+json` types), `[Form]`, `[Multipart]` (a
  binary field reads `<name>.bin`), then text. The value is the media
  type's example, else its first `examples` entry, else one generated from
  the schema.
- **Expected response:** `HTTP <lowest documented 2xx status>`.

Generated values are deterministic:

- A value comes from the schema's `example`, first `examples` entry,
  `const`, `default` or first `enum` value when one exists.
- Otherwise it is built from the schema:
  - **Objects:** required properties only.
  - **Arrays:** one item, or `minItems` items.
  - **Composition:** the first `oneOf`/`anyOf` branch; `allOf` branches are
    merged.
  - **Constraints:** numbers stay within their bounds; string lengths
    follow `minLength`/`maxLength`.
  - **String formats:** `email`, `uuid`, `date-time`, `date`, `uri`,
    `hostname`, `ipv4`/`ipv6` and similar get well-formed values.

`--group` lays the files out:

| Value | Layout |
|---|---|
| `tag` (default) | One directory per operation's first tag; untagged operations at the top. |
| `path` | One directory per first path segment (`/pets/{id}` → `pets/`). |
| `flat` | All files in the output directory. |

File names come from the operationId in kebab-case (`listPets` →
`list-pets.hurl`), or from the method and path without one.

The import also writes `sonde.yaml`, unless the output directory already
has one. Its `default` environment sets:

- `base_url`, from the first server;
- each path parameter, from its example.

When the spec lies inside the output directory, `sonde.yaml` also names it
as the project's `openapi.spec`, so `sonde test DIR` validates responses
with no flag.

Operations a file cannot be written for are listed as skipped, with the
reason. Request bodies of unsupported media types, such as
`application/octet-stream`, are left out with a warning.
