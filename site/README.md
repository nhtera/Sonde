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
npm --prefix site run build    # prerendered output in site/dist/
npm --prefix site run preview  # the built site, served by the Worker runtime
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
  `/api/search.json`, `/api/docs-tree.json`, `/404`. Output:
  - `site/dist/client/`: static assets, one `index.html` per page
    (`docs/getting-started/index.html`), `404/index.html`, the two JSON
    files;
  - `site/dist/server/`: the Worker (`index.js`, `wrangler.json`).
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
  `/docs/x` (`html_handling: drop-trailing-slash`); `/nope` returns status
  404 with the site's 404 page, rendered by the Worker. The Worker has no
  bindings and no secrets; it is about 0.6 MB gzipped.
- **Relative links (gate g).** Fumadocs' `createRelativeLink` needs the
  server-side source while rendering, but pages render from the browser
  collection, and it cannot send unpublished files to GitHub or enforce a
  scheme policy. Links are rewritten at build time by a remark plugin
  instead (`src/lib/remark-sonde-links.ts`).
- **No trailing slash.** URLs are `/docs/x`; `src/lib/urls.ts` builds every
  site URL.
- **Fumadocs colors.** `fumadocs-ui/css/neutral.css` is not imported (its
  `.dark #nd-sidebar` rule would beat the token mapping);
  `fumadocs-ui/css/lib/default-colors.css` registers the color utilities
  that `preset.css` needs, and the site maps them to the desktop tokens.
