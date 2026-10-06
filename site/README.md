# Sonde website

The landing page and the docs at https://sonde.erai.dev. Fumadocs on
TanStack Start (Vite 8), Tailwind CSS v4, built with the Cloudflare Vite
plugin. Every page is prerendered to HTML; a small Worker answers only the
requests no file matches (the 404 page).

## Run it

Node 22.18 or later.

```sh
npm --prefix site ci --ignore-scripts
npm --prefix site run dev      # http://localhost:3000
npm --prefix site run build    # output in site/.cloudflare/output/v0/
npm --prefix site run preview  # the built site, served by the Workers runtime
npm --prefix site test         # unit tests
npm --prefix site run test:browser  # axe, cascade, docs and search on the build (npx playwright install chromium)
```

From the repository root: `make site`, `make site-dev`, `make site-check`.

## Where the content comes from

`docs/` is the source of the docs. The site never edits it: a sync step
copies the published docs into `site/content/` (generated, git-ignored)
and the build renders them as Markdown. The look comes from the desktop
app's design tokens (`desktop/frontend/src/app/theme/tokens.css`), imported
as they are.

## Decisions from the spike

Recorded 2026-10-06 with `@tanstack/react-start` 1.168.60,
`@cloudflare/vite-plugin` 1.62.5, `fumadocs-mdx` 15.4.6,
`fumadocs-ui`/`fumadocs-core` 16.16.2, `vite` 8.3.2.

- **Markdown, not MDX (gate a: passed).** fumadocs-mdx compiles `.md` files
  with `format: 'md'`. On the hostile fixture (`test/fixtures/hostile.md`):
  `{process.env.HOME}`, `{{base_url}}` and `Map<string, number>` render as
  text; `<script>`, `<img onerror>`, `<div onclick>` and HTML comments are
  dropped (no `rehype-raw`). A `javascript:` link is neutralized by React;
  the docs links plugin also fails the build on it.
- **Highlighting (gate b: passed).** The repository's TextMate grammar
  (`editors/vscode/syntaxes/sonde.tmLanguage.json`) is registered as `hurl`
  with the alias `sonde`, replacing Shiki's bundled `hurl`. Bundled
  languages (sh, json, yaml, go, csv, lua) load on demand.
- **Code colors (gate c).** One Shiki theme (`src/lib/shiki.ts`) whose
  colors are the desktop syntax variables (`var(--t-*)`), mapped like the
  desktop editor. Light and dark follow the theme toggle with no second
  theme.
- **Prerender (gate d: passed).** `crawlLinks` finds every linked page.
  Paths no page links to are listed in `vite.config.ts` `pages`:
  `/api/search.json`, `/api/docs-tree.json`, `/404.html`, `/sitemap.xml`,
  `/robots.txt`. Output (Cloudflare Build Output, phase 5):
  - `.cloudflare/output/v0/workers/default/assets/`: one `index.html` per
    page (`docs/getting-started/index.html`), `404.html`, the JSON files,
    `_headers`. Every check runs on this directory (`scripts/paths.mjs`);
  - `.cloudflare/output/v0/workers/default/bundle/`: the Worker.
- **Search (gate e: passed).** `/api/search.json` is a prerendered file (an
  Orama export); the ⌘K dialog downloads it on first open and searches in
  the browser. The index is not in the Worker bundle.
- **Client navigation without a server.** A docs page needs the page tree.
  During prerender the docs route calls a server function in-process; in
  the browser it reads the prerendered `/api/docs-tree.json` instead
  (`src/lib/docs-data.ts`). `@tanstack/start-static-server-functions` was
  not used: it writes its cache files from the prerender process, which
  runs in workerd and cannot write to the build output.
- **Worker scope (gate f: passed, under `vite preview`).** Prerendered
  paths and the JSON files are served as assets; `/docs/x/` redirects to
  `/docs/x` (`drop-trailing-slash`). Assets are served before the Worker,
  so the deployed Worker (`src/server.ts`) only sees paths with no page: it
  answers with `404.html`, status 404 and the security headers. TanStack
  renders only for prerender requests (a per-build token header) and in
  development. Its one binding is `ASSETS`; no secrets. About 0.8 MB
  gzipped. (`not_found_handling: "404-page"` does not apply while a Worker
  exists: misses go to the Worker.)
- **404 page.** `404.html` is made static after the build
  (`scripts/static-404.mjs`): it is served at any URL, where hydrating would
  put the router in its not-found state and re-render. It has only links.
- **Relative links (gate g).** Fumadocs' `createRelativeLink` needs the
  server-side source while rendering, but pages render from the browser
  collection, and it cannot send unpublished files to GitHub or enforce a
  scheme policy. Links are rewritten at build time by a remark plugin
  instead (`src/lib/remark-sonde-links.ts`).
- **CSP.** `scripts/csp-headers.mjs` writes `_headers` after the build:
  security headers for every path, immutable caching for `/assets/*` and
  `/screens/*`, and one `Content-Security-Policy` per page with the SHA-256
  of each inline script (no `'unsafe-inline'` for scripts). Scripts are
  hashed as the browser parses them: TanStack's hydration data contains a
  NUL, which the HTML parser turns into U+FFFD. Cloudflare reads at most 100
  `_headers` rules; the step fails before that (45 today).
- **Search loads on demand.** The search dialog and its index client are a
  lazy chunk; nothing about search is on the critical path.
- **No trailing slash.** URLs are `/docs/x`; `src/lib/urls.ts` builds every
  site URL.
- **Fumadocs colors and CSS.** Neither `neutral.css` (its `.dark
  #nd-sidebar` rule would beat the token mapping) nor
  `lib/default-colors.css` is imported: `src/styles/sonde-site.css` defines
  the `--color-fd-*` theme itself, from the desktop tokens. Of `preset.css`,
  only the docs layout's parts are imported (`src/styles/fumadocs.css`).

## Deploy

`.github/workflows/site.yml`, with the `cf` CLI (beta, pinned):

- `site-build` (pull requests that touch the site's inputs, and `main`):
  no secrets. Installs without scripts, runs the npm checks, lint,
  typecheck and tests, `npm run sync`, `npx cf build`, `npm run finalize`,
  the link check and the browser tests on the output, checks that the
  build changed no file, and runs `cf deploy --prebuilt --dry-run` (no
  credentials needed). On `main` it uploads `.cloudflare/output/v0`.
- `site-gate`: always runs; the required check.
- `site-deploy` (`main` only, environment `site`): installs only `cf` from
  `site/deploy/`, deploys the uploaded output with `CLOUDFLARE_API_TOKEN`
  on that one step, then runs `site/deploy/smoke.sh`.
- A finished `release-desktop` run starts a run on `main` (`workflow_run`),
  which refreshes the Download link: the newest stable `desktop/v*` tag
  that has a published release (CI lists them with `gh`; the build job
  passes the list to sync in `SONDE_SITE_RELEASES_FILE`). `on: release`
  would never fire: releases are created with `GITHUB_TOKEN`.
- `DO_NOT_TRACK=1` turns off the `cf` CLI's telemetry in CI.

One-time setup (done 2026-10-06): the account-owned API token
`sonde-site-deploy` with Workers Scripts:Edit only, expiring 2027-10-06
(rotate yearly); `CLOUDFLARE_API_TOKEN` and
`CLOUDFLARE_ACCOUNT_ID` as secrets of the GitHub environment `site`
(deployment branches: `main`); after the first deploy, attach the custom
domain `sonde.erai.dev` to the `sonde-site` Worker (Settings → Domains &
Routes); require `site-gate` in branch protection. The first deploy's smoke
step fails until the domain is attached; re-run the job after attaching it.

## Rollback

Tried on production on 2026-10-06 (v1 → v2 → v1 → v2, the site served
200 throughout). With a Cloudflare login (`npx cf auth login`) or
`CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` in the environment:

```sh
cd site/deploy && npm ci --ignore-scripts
npx cf workers deployments list --worker sonde-site      # the active version
npx cf workers versions list --worker-id sonde-site      # pick the previous version id
npx cf workers deployments create --worker sonde-site --strategy percentage \
  --versions '[{"version_id":"<previous-id>","percentage":100}]'
```

With Wrangler: `npx wrangler rollback --name sonde-site`. Roll forward by
deploying the newer version id the same way, or by re-running the `site`
workflow on `main` (workflow_dispatch).

## Fallback: Wrangler

If a `cf` beta breaks the build or deploy, use the GA path: replace
`@cloudflare/vite-plugin` with `1.62.5` and `cloudflare.config.ts` with
`wrangler.jsonc`:

```jsonc
{
  "name": "sonde-site",
  "compatibility_date": "2026-10-01",
  "compatibility_flags": ["nodejs_compat"],
  "main": "./src/server.ts",
  "assets": { "binding": "ASSETS", "html_handling": "drop-trailing-slash" }
}
```

Build with `vite build` (output in `dist/client` and `dist/server`), and
deploy with a pinned `wrangler` in `site/deploy/` (`wrangler deploy`).
