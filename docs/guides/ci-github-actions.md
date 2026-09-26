# Running tests in GitHub Actions

Run every request file of a directory in test mode and keep the reports.

```yaml
name: api-tests
on: [push, pull_request]
permissions:
  contents: read
jobs:
  api-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/nhtera/sonde/cmd/sonde@latest
      - name: Run API tests
        env:
          SONDE_SECRET_token: ${{ secrets.API_TOKEN }}
        run: |
          sonde --test --jobs 4 \
            --variable base=https://staging.example.com \
            --report-junit build/junit.xml \
            --report-html build/html \
            tests/api
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: api-test-reports
          path: build/
```

- `--test` runs files in parallel (`--jobs` sets how many at a time) and
  prints one line per file and a summary; the exit code is non-zero when a
  file fails.
- A directory argument runs every `*.hurl` file under it, in sorted order.
- Secrets come from `SONDE_SECRET_<name>` environment variables (or
  `--secret name=value`); their values are redacted from the output and
  from every report.
- With a `sonde.yaml` in the repository, select an environment with
  `--env staging` or `SONDE_ENV=staging` (see [sonde.yaml](../sonde-yaml.md)).
- Colors are off when the output is not a terminal; `NO_COLOR` also turns
  them off.
