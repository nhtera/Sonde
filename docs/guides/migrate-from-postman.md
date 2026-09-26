# Migrating from Postman

`sonde import postman COLLECTION.json -o DIR` converts a Postman Collection
(v2.1, and v2.0, which parses the same way) into Sonde request files: one
`sonde import` run, then edit the result like any other request file. This
guide covers what maps across, what the importer warns about instead, and
how to run the result. The common import flags (`-o`, `--ext`, `--force`,
`--dry-run`, naming, collisions) are in
[import-export.md](import-export.md); this page only covers what is
specific to `postman`.

A v2.0 export's few shape differences from v2.1 are tolerated, not just its
schema URL: `auth` params written as an object (`{"username": "u", ...}`)
instead of a `[{key, value}, ...]` list, a request written as a bare URL
string, and a `header` written as one raw `"Key: value\nKey2: value2"`
string all import the same as their v2.1 equivalent.

```sh
sonde import postman collection.json -o requests/
$EDITOR requests/*.hurl                    # fill in real values, add asserts
sonde --test --env collection requests/    # run with the collection's variables
```

## Layout

- `--group request` (the default): every request becomes one file, named
  after it; a folder becomes a directory. A request directly in a nested
  folder ends up at `folder/subfolder/request-name.hurl`.
- `--group folder`: every folder becomes **one file**, chaining its own
  requests in order (a nested folder's requests are not repeated in its
  parent's file: the nested folder gets its own file). Requests directly
  on the collection (not in any folder) become one file named after the
  collection.

Either way, a disabled request is skipped (reported in the summary), and
an empty folder produces nothing.

## Variables and environments

A Postman collection's own `variable` array, and every folder's, become
one flat set of variables: a folder's variable overrides one of the same
name from the collection or an earlier sibling folder. This is a
deliberate simplification — Sonde's `sonde.yaml` environments have no
per-directory scope the way a Postman folder does, so nesting is
flattened rather than dropped. These become the `collection` environment
of the generated `sonde.yaml`.

Every `--environment FILE` (repeatable) adds one more environment, named
after the file's own `"name"` (falling back to the file's base name),
lowercased and hyphenated (ASCII only: unlike a generated file's path
segments, a non-ASCII letter does not survive). Its variables are
the collection's, overridden by the file's own enabled values — each
environment is self-contained, usable on its own without selecting
`collection` first. `defaults.env` is the **first** `--environment` given,
or `collection` when none was:

```sh
sonde import postman collection.json -o requests/ \
  --environment dev.postman_environment.json \
  --environment prod.postman_environment.json
# sonde.yaml: environments "collection", "dev", "prod"; defaults.env: dev
```

A variable typed `secret`, in the collection or in an environment file, is
never written with its value: only its name goes into a secrets stub file
(`secrets/<environment>.secrets`, referenced from that environment's
`secrets_files`), one `name=` line per secret, ready to fill in — and a
warning names it. That file's own name comes from the writer's own path
sanitizer (the same one every generated request file's path goes through,
docs/guides/import-export.md), which is not always identical to the
environment's own name above for a non-ASCII one. Fill the value in, or
override it with `--secret name=value` at run time; either way, no secret
value from the source collection ever reaches disk.

Two different Postman variable names that sanitize to the same Sonde name
(`api.key` and `api_key` both become `api_key`, say) produce a warning
naming both; the later one, in document order, wins.

Every `{{variable}}` placeholder — in a URL, header, body, or auth
field — becomes a Sonde `{{variable}}` the same way, its name made valid
the same way a declared variable's is, so the two always match. A few
Postman dynamic variables translate to the matching Sonde function
(`{{$uuid}}`, `{{$guid}}` → `newUuid`; `{{$isoTimestamp}}` → `newDate`);
any other `{{$...}}` becomes a plain variable to set, with a warning.

## Auth

Basic, bearer and API key auth translate directly and work: bearer becomes
an `Authorization: Bearer ...` header, API key becomes a header or a query
parameter depending on its `in`.

Digest, NTLM and AWS SigV4 map to `[BasicAuth]` plus the matching
`[Options]` flag (`digest`, `ntlm`, `aws-sigv4`; an AWS session token
becomes an `X-Amz-Security-Token` header) — but **Sonde cannot send any of
the three yet**: the mapping is kept, for when it can, but the request
fails at run time until then (a warning says so at import time too, so it
is not a surprise later). Every other type — OAuth 1/2, Hawk, EdgeGrid,
JWT, ASAP, or anything unrecognized — has no Sonde equivalent at all and is
warned about by request name; add it by hand.

Auth is inherited the way Postman inherits it: a request's own `auth`
wins; failing that, its nearest enclosing folder's; failing that, the
collection's. `"noauth"` is an explicit auth of its own — it stops
inheriting from anything above it, exactly like setting a real auth
would.

## Bodies

- `raw` with `options.raw.language: json`: parsed and rebuilt, keeping
  member order and duplicate keys exactly as written, so a `{{variable}}`
  inside a JSON string value keeps working. When the text is not valid
  JSON (most often because a placeholder sits outside a string, e.g.
  `{"count": {{n}}}`, which is not valid JSON syntax — or inside an object
  *key*, which Sonde's body builder never rebuilds), it is kept as a
  templated body instead of structured JSON — every `{{variable}}` in it,
  including one in a key, still substitutes, but a warning notes the body
  is no longer valid JSON on its own.
- `raw` with `xml`, `text`, `html`, `javascript`, or no language at all:
  a templated body, so every `{{variable}}` in it keeps substituting
  (`text`/`html`/`javascript` have no matching Sonde body language of
  their own, so they become a plain templated body rather than a
  language-tagged one; the inferred `Content-Type` header still reflects
  the original language).
- `urlencoded` → `[Form]`; `formdata` → `[Multipart]` (a file field keeps
  only its file name, as a reference: Import never reads or copies the
  file).
- `graphql` → a real Sonde GraphQL body: the query keeps its
  `{{variable}}` placeholders working, and a `variables` object becomes a
  genuine `variables { ... }` block (or, when the query cannot be written
  that way, the equivalent `{"query": ..., "variables": ...}` JSON body —
  either way a real, working GraphQL request). A `variables` value present
  but not a JSON object is dropped, with a warning, rather than guessed
  at.
- `file` → a file body (its source path, as a reference, same as a
  multipart file field).
- Anything else is warned about and left without a body.

A disabled header, query parameter, form field or multipart field is
skipped, same as a disabled request.

## Scripts

Pre-request and test scripts are **never executed** — Sonde has no
JavaScript runtime, and would not run untrusted script from an imported
file even if it did. Every non-empty script becomes a `#`-commented block,
plus one warning per script, so every kept-but-inert script is easy to
find with `grep`. A request's own scripts land on its own file. A folder's
or the collection's have no file of their own to go on: with `--group
folder` they go on the first entry of the file that folder (or the
collection, for its own top-level requests) produces; with `--group
request` (the default) they go on the first request built after them, in
document order, wherever it ends up.

The one exception: a `test` script's `pm.response.to.have.status(N)` or
`pm.expect(pm.response.code).to.eql(N)` / `.to.equal(N)` — nothing more —
is recognized and translated to a real `HTTP N` expectation. Every other
assertion stays exactly what it was: inert text in a comment.

## Running the result

`sonde check requests/` parses every generated file; `sonde --test --env
collection requests/` runs them against wherever `base_url` (or whatever
the collection called it) actually points, with the collection's
variables and secrets stub. Override anything from the command line the
same way you would for a hand-written project:

```sh
sonde --test --env prod --variable base_url=https://staging.example.test \
  --secret token=... requests/
```

A warning does not fail the import (exit 0); an unreadable collection, an
unsupported `--group` value, or a Collection v1 export (re-export it as
v2.1) does (exit 1). Nothing is executed from the collection at import
time: no script, no network request.
