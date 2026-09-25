# Sonde

![status: pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange)

**Sonde** (pronounced "sond") is a fast, single-binary CLI that runs and tests
HTTP requests written in plain text. It reads [Hurl](https://hurl.dev) 8 files
(`.hurl`, and `.sonde` for the same syntax) and adds OpenAPI contract checks,
data-driven runs, environments, importers and editor support.

> Pre-alpha: only `sonde version` works today. The example below shows where
> the project is heading.

## Install

```sh
go install github.com/nhtera/sonde/cmd/sonde@latest
```

## Example

```hurl
GET https://example.org/api/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
```

```sh
sonde --test health.hurl
```

## Documentation

See [docs/](docs/README.md).

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE) and
[TRADEMARKS.md](TRADEMARKS.md) for use of the name.
