# Data-driven runs

`--data FILE` runs every request file once per row of a CSV or JSON data
file; each column becomes a variable. Request files are unchanged, so they
stay runnable without the flag (give the variables with `--variable`).

```hurl
# login.hurl
POST {{base}}/login
{
  "user": "{{user}}",
  "password": "{{password}}"
}
HTTP {{status}}
```

```csv
user,password,status
alice,correct-horse,200
bob,wrong,401
```

```sh
sonde --test --variable base=https://staging.example.com \
  --data users.csv --data-secret password login.hurl
```

```
Success login.hurl#row-1 (1 request(s) in 41 ms)
Success login.hurl#row-2 (1 request(s) in 38 ms)
```

## Data files

The format follows the extension; any other extension is an error.

- **CSV** (`.csv`, RFC 4180): the first row names the columns; blank or
  duplicate names are errors. A UTF-8 byte order mark is ignored, quoted
  cells may hold commas, quotes (`""`) and line breaks, and an empty cell
  is the empty string. Values are typed like `--variable` values: `true`,
  `false`, `null`, integers and floats become those types, `"quoted"`
  text is a string, anything else is a string.
- **JSON** (`.json`): an array of objects, read one object at a time.
  Values keep their JSON type, including objects and arrays. An object may
  leave a column out.

The data file must be a regular file. The whole file is checked before
the first request, so a malformed row fails the run (exit 1) before
anything is sent. Rows are then read again from disk for each request
file, so memory does not grow with the number of rows. If the file changes
during the run and a row then fails to read, the run stops with exit 1
and writes no reports.

## Variables

- `data_row` is the 1-based row index. A column cannot be named
  `data_row`.
- Precedence, lowest first: `sonde.yaml`, `HURL_VARIABLE_*`/`SONDE_VARIABLE_*`,
  `--variables-file`, **the data row**, `--variable`, entry options,
  captures. A `--variable` therefore replaces a column for every row.
- A column cannot share its name with a secret (`--secret`,
  `--secrets-file`, `*_SECRET_*` environment variables, the selected
  `sonde.yaml` environment's `secrets_files`) nor with a template function
  (such as `newUuid`).
- Values are typed exactly like `--variable` values, so `007` is the
  integer 7; write the cell as `"""007"""` (a quoted `"007"`) to keep it
  as text.
- In JSON, an object that repeats a key keeps the last value.

## Secrets

`--data-secret COL[,COL...]` (repeatable) marks columns as secrets: their
values are redacted like other secrets from the terminal, `--json`,
`--curl`, the cookie jar and every report. A secret cell must be text, a
number or a boolean; a JSON `null` sets no secret.

A row's secrets, and the `redact` captures made while running that row,
are redacted from that row's own output and results only, so a run over
many rows does not accumulate every row's secrets. Output of another row
is not scanned for them.

## Test mode, reports and repeats

- Every request file runs for every row, file by file:
  `a.hurl#row-1 … a.hurl#row-N`, then `b.hurl#row-1 …`. With `--test`,
  the rows run in parallel like files (`--jobs`).
- Each row is a separate unit in the summary and in reports, labeled
  `<file>#row-<N>` (JUnit test case, TAP line, HTML page). The JSON result
  keeps `filename` and adds `"sonde": {"iteration": {"row": N}}`
  (see [report-json.md](../report-json.md)).
- `--repeat N` repeats the whole file × row sequence.
- A data file with no rows runs nothing and prints a warning.
