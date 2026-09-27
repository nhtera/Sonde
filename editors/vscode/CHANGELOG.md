# Changelog

All notable changes to the Sonde VS Code extension are documented here.

## Unreleased

- `.sonde` files get a Sonde file icon wherever the file icon theme has none of its own.

## 1.0.1

- Sharper 256px Marketplace icon, drawn from an SVG source.

## 1.0.0

- Marketplace readiness: publisher `nhtera`, an icon, `repository`/`bugs`/`homepage` metadata, `untrustedWorkspaces` (limited, `sonde.path` restricted) and `virtualWorkspaces: false` capabilities.
- Bundled with esbuild (`vscode:prepublish`): the packaged `.vsix` now ships one `dist/extension.js` instead of the full `node_modules` + compiled source tree.
- Release workflow: tags `editors/vscode/v*` publish to the VS Code Marketplace and attach the `.vsix` to a GitHub release.

## 0.1.0

- Initial release: diagnostics, completion, hover and formatting for `.sonde` files via `sonde lsp`, with optional `.hurl` association.
