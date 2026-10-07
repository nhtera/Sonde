# 0002: OpenCollection import mapping

Status: Accepted. Date: 2026-09-26.

## Context

Phase 8 adds `sonde import opencollection INPUT`: convert a Bruno
OpenCollection YAML collection (a single file or a directory) to Sonde
request files. OpenCollection is young and still moving (the phase's own
risk register flags this); this record pins the version supported, the two
input layouts accepted, and the field-by-field mapping so a future spec
bump is a diff against this document rather than a re-discovery.

Sources consulted (2026-09-26): [opencollection.com](https://www.opencollection.com/)
(spec overview, version, `bundled` flag), the OpenCollection JSON Schema at
`packages/oc-schema/src/opencollection.schema.json` in
[github.com/opencollection-dev/opencollection](https://github.com/opencollection-dev/opencollection)
(authoritative field list — draft-07 JSON Schema, `additionalProperties:
false` on most objects), and Bruno's own docs at
[docs.usebruno.com/opencollection-yaml/overview](https://docs.usebruno.com/opencollection-yaml/overview)
(Bruno 3.0+ ships OpenCollection YAML as an alternative to `.bru`; its
directory tree shows the `opencollection.yml` root file, one `.yml` file
per request, a `folder.yml` per folder and an `environments/` directory).

## Pinned version

**OpenCollection spec 1.0.0** (`opencollection: "1.0.0"` top-level key).
This importer reads any `1.x` document; a different or missing
`opencollection` value is a warning (`unsupported`, "unrecognized
opencollection version %q, importing as 1.x best-effort"), not a failure —
matching the phase's "unknown fields/versions: warning, not failure" rule.
Every unrecognized field anywhere in the document is silently ignored (the
importer's Go structs only declare the fields it maps; YAML decoding is
tolerant of the rest) rather than erroring, since `additionalProperties:
false` in the schema is a *producer*-side contract Sonde does not enforce
on *input*.

## Input layouts

### Single file

One YAML document matching the schema's root shape:

```yaml
opencollection: "1.0.0"
info: {name: "...", version: "..."}
config:
  environments: [{name: "Local", variables: [...]}]
request: {headers: [...], auth: {...}, variables: [...]}   # collection defaults
items:
  - info: {name: "Get user", type: http, seq: 1}
    http: {method: GET, url: "{{baseUrl}}/users/1"}
  - info: {name: "Users", type: folder, seq: 2}
    request: {auth: {...}}                                  # folder defaults
    items: [...]
```

`items` nests folders (`info.type: folder`, itself carrying `items`) and
leaf requests arbitrarily deep. `bundled: true` (or the field's absence) is
assumed for a single file; `bundled: false` is accepted too (there is
nothing left to bundle once the whole document is one file) and does not
change behavior.

### Directory

A directory containing:

- `opencollection.yml` or `.yaml` at its root: the same top-level shape as
  the single-file case, but with `items` omitted or empty — the tree below
  the directory supplies the items instead.
- One `<name>.yml`/`.yaml` file per request, matching a leaf item's shape
  directly (`info`, `http`/`graphql`, `runtime`, `settings`).
- One subdirectory per folder. A subdirectory may contain a `folder.yml`/
  `.yaml` holding that folder's own `info` (name override), `request`
  (defaults) and `docs`; without one, the directory's base name is the
  folder's name and it has no defaults of its own. Bruno's OpenCollection
  YAML overview (docs.usebruno.com/opencollection-yaml/overview) shows
  this layout: `opencollection.yml` at the root, `folder.yml` in each
  folder directory, one `.yml` file per request, and environments under
  `environments/`.
- Dotfiles and dot folders (`.git`, `._` resource forks), `__MACOSX` and
  `node_modules` are no part of the collection and are skipped.

### Zip

A zip of a directory (the ZIP export), known by its bytes whatever its
name, is read in place and never extracted. The collection is the zip's
root or, when the root holds only a folder with an `opencollection.yml`,
that folder. The directory limits apply to the sizes the zip declares.
- Environments: either inline in the root file's `config.environments`
  (as in the single-file case), or, if present, one `<name>.yml`/`.yaml`
  file per environment under an `environments/` subdirectory, each holding
  an `Environment` object body directly. Both are read; a name defined in
  both is a `WarnUnsupported` ("environment %q defined twice; using the
  environments/ file") in favor of the file.
- Any other top-level file or directory entry (an unrecognized name, a
  dotfile, a non-YAML file) is ignored, not an error.

Traversal is done through `os.OpenRoot(dir).FS()` (never `os.Open`
directly), so a symlink inside the directory cannot walk the import
outside it — that entry is skipped with a warning rather than aborting
the whole import, since only that one file or subdirectory is unusable;
only regular files are read otherwise. Every file is capped at 16 MiB,
the whole directory at 256 MiB total, 10,000 files and 10,000
directories, and directory nesting at 64 levels (all four, `dirread.go`'s
`budget`), as `docs/guides/import-export.md` states. The file/byte caps bound
the content, the directory count and depth caps bound the tree shape
itself, which a directory symlink cycle entirely *inside* the collection
directory could otherwise grow or deepen without limit (`os.Root` only
refuses a symlink that leaves the directory, not one that cycles within
it). A single-file collection is capped the same way: at most 16 MiB,
the same per-file limit the directory layout gives each of its files,
checked by `ImportFile` itself even though the CLI's own read may allow
up to `convert.MaxInput` (64 MiB) before handing the bytes over. YAML
decoding goes through `go.yaml.in/yaml/v3`'s `Decoder`, whose alias
handling already refuses a document whose alias-to-node ratio blows up
within *one* decode call (its built-in "billion laughs" guard,
`decode.go`'s `aliasCount`/`allowedAliasRatio`) with a decode error this
importer treats like any other malformed file. That guard does not carry
over across the separate `yaml.Node.Decode` calls this importer makes to
decode each item independently (`decode.go`'s `decodeItemNode`, for
per-item error isolation): a self-referential
anchor there is instead bounded by this importer's own `maxItemDepth`
(200 levels of "items" nesting) — found, and fixed, via this package's
own fuzz corpus before it shipped.

**Ordering.** `info.seq` (a positive integer), when present on a folder or
request, orders items relative to their siblings, ahead of any sibling
without one; ties, and every item with no `seq` at all, sort by name
(`sortItems`, `tree.go`) — the same rule in both layouts, not "file name
in a directory, document order in a single file" as an earlier draft of
this record said: `info.name` is what both layouts actually compare.
Order only affects *which* item gets an unsuffixed output path when two
items sanitize to the same name — Sonde file names always come from the
request's own name (kebab-cased by the shared writer), never from `seq`,
so `seq` is not baked into a file name verbatim.

## Field mapping

### Item kinds

| OpenCollection `info.type` | Sonde |
|---|---|
| `http` | one request file |
| `graphql` | one request file: `POST`, body built with `syntax.GraphQLBody(query, variables)` — a `{{variable}}` in the query, or inside a `variables` value, stays live; `variables` must decode to a JSON object (anything else is a `WarnUnsupportedBody`, sent without variables); when `GraphQLBody` decides the query can't be written as a ` ```graphql ` block, it sends the equivalent `{"query": ..., "variables": ...}` JSON body instead — the same thing a GraphQL request sends on the wire either way, so this is not itself a warning |
| `folder` | a subdirectory; not a request file itself |
| `grpc`, `websocket`, `script`, `app` | **skipped** (`Skipped`, reason "opencollection: <type> items are not requests Sonde can import"); Sonde's file format has no gRPC/WebSocket/script/app request shape (`internal/grpcx`, `internal/stream` are post-v1, and `internal/convert` cannot depend on them regardless) |

### Request fields (`http`/`graphql` block)

| OpenCollection | Sonde |
|---|---|
| `method`, `url` | request line (a `/` in an item or folder name is part of the file name, never a folder); `url` through `convert.ParseText` for `{{var}}` |
| `headers[]` (`disabled: true` dropped) | header lines, value through `ParseText` |
| `params[]` with `type: query` (`disabled` dropped) | `[Query]` field, unless `url` has its own query string: an export writes the query in both places, so the URL keeps it and the request sends each param once |
| `params[]` with `type: path` | not written as its own section (Sonde has none); the OpenCollection convention of a `:name` URL segment is rewritten to `{{name}}` in the URL and the param's `value` becomes an `[Options] variable: name="value"` default, so the request still runs standalone |
| `body.type: json` | `convert.JSONBody` (shared with the other importers): one JSON value rebuilt keeping member order, duplicate keys and every `{{variable}}` inside a string value live; malformed JSON, deep nesting, or a placeholder used where JSON syntax does not allow one (a bare `{{amount}}` in a numeric position, or one inside an object *key*) falls back to `convert.ParseText` + `syntax.TextBody(t, "json")` — the original text verbatim, but `{{variable}}` anywhere in it, including those positions, still expands live; no warning either way, since both paths keep every placeholder working |
| `body.type: xml/text/sparql` | `convert.ParseText` + `syntax.TextBody(t, lang)` (`lang` "xml" for xml, "" for text/sparql — OpenCollection gives text/sparql no templating story of their own and Sonde has no distinct lang tag for them): the text verbatim with every `{{variable}}` live, same as the JSON fallback above |
| `body.type: form-urlencoded` (`disabled` dropped) | `[Form]` |
| `body.type: multipart-form` (`disabled` dropped) | `[Multipart]`: `type: text` → text field, `type: file` → `file,PATH;TYPE` |
| `body.type: file` | `file,PATH;TYPE` from the entry marked `selected: true`, else the first entry; more than one entry is a `WarnUnsupportedBody` ("only the selected file is imported") |
| `body` variants array (`variants: [{title, selected, body}]`) | only the `selected: true` variant is imported (or the first, none marked); a `WarnUnsupportedBody` names the ones dropped |
| `settings.*` (`encodeUrl`, `timeout`, `followRedirects`, `maxRedirects`, `omitHeaders`) | `WarnUnsupportedOption` per field set to other than the export default (`encodeUrl: true`, `timeout: 0`, `followRedirects: true`, `maxRedirects: 5`, `omitHeaders: false`) — no Sonde `[Options]` equivalent maps 1:1 without changing request semantics, so these are surfaced rather than guessed at; the defaults, written into every exported request, import silently |
| `docs` | a `#` comment line above the request (`Description` may be a plain string or `{content, type}`; the `content` is used, `type` is ignored) |

### Auth (`http.auth`/`graphql.auth`, inherited through folders)

Resolution walks from the item up through its enclosing folders (directory
layout: parent directories with a `folder.yml`; single-file layout: the
enclosing `items` ancestors) to the collection root's `request.auth`,
using the **first** auth found starting at the item itself; `auth:
"inherit"` (the literal string) or an absent `auth` both mean "keep
looking upward". `auth: "none"` is different: it means "no auth here, and
stop looking upward too" — a deliberate override, not a gap to warn about
— resolving to no auth section at all, the same as if nothing in the
chain had set one. The resolved auth, if any, is written directly on the
request (Sonde files are flat — there is no folder-level auth section to
inherit *from* at read time).

| OpenCollection `auth.type` | Sonde |
|---|---|
| `basic` | `[BasicAuth]` |
| `bearer` | `Authorization: Bearer {{token}}`-shaped header, `token` through `ParseText` |
| `apikey` | `placement: header` → a header named `key`; `placement: query` → a `[Query]` field named `key`; key and value through `ParseText` |
| `none` | no auth section, no warning (an explicit override, not an unsupported scheme) |
| `digest` | the `user` option (`username:password`) and `digest: true` |
| `ntlm`, `wsse`, `awsv4`, `oauth1`, `oauth2` (any flow) | `WarnUnsupportedAuth`, naming the scheme; the request is written without auth |
| no `auth` anywhere in the chain | no auth section |

### Variables and environments

- Collection-level `request.variables` and every folder's `request.variables`
  on the path to an item, plus the item's own `runtime.variables`, are
  merged (leaf wins on a name collision) into one `default` sonde.yaml
  environment (`config.EmitProject`, `defaults.env: default`) — the same
  pattern `sonde import openapi` uses for its generated environment.
- Each `config.environments[]` entry becomes its own sonde.yaml
  environment, seeded with the merged `default` layer above (a name it
  does not itself declare still reaches it, the way Postman's importer
  already seeds every generated environment with the collection's own
  variables) and named after `Environment.name` — except `default`
  itself, which is reserved for the generated environment above: an
  OpenCollection environment named `default`, or one that collides with
  another already-named environment, is renamed (`" (2)"`, `" (3)"`, ...)
  with a `WarnUnsupported` saying so, rather than silently overwriting it.
- Every secrets-stub path this importer writes, and the matching
  `secrets_files` entry, comes from the same call to `convert.StubPath`
  (one shared `used` map across every environment, `environment.go`), so
  the two can never point at different sanitized names the way two
  independently-sanitized copies of the same logic could drift apart.
- A `Variable.value` that is a plain string is used as is; `{type, data}`
  uses `data`; a variant array (`[{title, selected, value}]`) uses the
  `selected: true` entry, or the first if none is marked, with a
  `WarnUnsupported` naming the dropped variants when there was more than
  one.
- A `SecretVariable` (`secret: true`) **never** has its value written —
  the OpenCollection schema itself gives this shape no `value` field, so
  there is nothing to redact by omission, only a name to register. Every
  secret variable becomes a `name=` line in a secrets-stub `RawFile{Keep:
  true}` (one stub per sonde.yaml environment that declares any, plus one
  for collection/folder-level secrets feeding the `default` environment),
  referenced from that environment's `secrets_files`, plus one
  `WarnSecret` per secret naming it.
- `externalSecrets` (Vault/AWS/Azure/GCP-backed secrets) has no Sonde
  equivalent at all (Sonde does not call out to a secrets manager at
  import or run time): its presence on an environment is one `WarnSecret`
  naming that environment (not each of its variables individually — the
  schema's own per-provider shape is not parsed any further than
  detecting the key exists), and contributes no line to any secrets stub.
- `dotEnvFilePath`, `extends`, `clientCertificates` on an environment, or
  `config.proxy` naming a host or `config.protobuf` on the collection (an empty proxy block, with no host, is no proxy): `WarnUnsupported`,
  no Sonde equivalent.

### Scripts, tests and assertions — never executed

`runtime.scripts` (any `type`: `before-request`, `after-response`,
`tests`, `hooks`, the `grpc:*` lifecycle stages) are **never executed**.
Each script's code is kept as `#`-comment lines above the request,
prefixed with its stage (`# opencollection before-request script:`, one
comment block per script, in document order), plus one `WarnScript` per
script naming its stage.

The app then offers, as suggestions to accept one by one (package
`internal/convert/suggest`, shared with the other collection importer), what the
`after-response` and `tests` scripts translate to: `expect(X).to.equal(V)`
(also `eql`, `deep.equal`) on `res.getStatus()`/`res.status`,
`res.getHeader(name)` or the body (`res.getBody()`/`res.body`, or a
`const` alias of it, with property and index accessors) becomes an assert;
`bru.setVar`/`setEnvVar`/`setGlobalEnvVar`/`setCollectionVar` of the body
becomes a capture. A statement counts when it always runs (top level, or
in a `test` callback); a capture also counts in an `if` that holds for a
2xx status only (`< 201..300`, `<= 200..299`, `== 200..299`). Anything
else stays a comment.

`runtime.assertions` (declarative `{expression, operator, value}`, no code
to run) are translated only for the one trivially safe pattern this phase
scopes for status checks: an assertion whose `expression` is `res.status`,
`response.status` or `res.statusCode` (the spelling varies across the
sources found) and whose `operator` is `eq`/`equals`/`==` and whose `value`
parses as an HTTP status code (100–599) becomes the entry's `HTTP N` line.
At most one assertion is translated this way (the first match); every
other assertion, and every status assertion once one has already been
used, becomes a `#` comment (`# opencollection assertion: EXPRESSION
OPERATOR VALUE`) plus a `WarnScript`. `runtime.actions` (`set-variable`)
have no declarative-to-Sonde translation at all (they run relative to a
live response) and are always comments + `WarnScript`.

## Unsupported entirely

`grpc`/`websocket`/`script`/`app` items (above), OAuth1/OAuth2 and the
other long-tail auth schemes, `externalSecrets`, `clientCertificates`,
`config.protobuf`, `config.proxy`, per-request `settings.*`, and
`examples` (recorded example responses — informational only, nothing in
Sonde reads them back). All of these are named warnings or skips, never a
failure: a document using only unsupported features still imports (as an
empty or near-empty output) rather than erroring, matching "unknown
fields/versions: warning, not failure."

## Consequences

- The `environments/` directory and `folder.yml`/`.yaml` conventions match
  what Bruno's own OpenCollection YAML overview shows (§Context); if a
  real Bruno-exported directory collection ever turns out to use a
  different shape in practice, only `internal/convert/opencollection`
  needs to change — the mapping table above stays valid regardless, since
  it describes the YAML shapes themselves, not their file names.
- If OpenCollection reaches a `2.0.0` with breaking shape changes, this
  importer's version check degrades to "best-effort" import with a
  warning rather than refusing the file outright; revisit the pinned
  version and this table together when that happens.
