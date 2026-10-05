// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Serves the prerendered site (dist/client) the way Cloudflare's static
// assets do with `html_handling: drop-trailing-slash`: /docs/x is
// docs/x/index.html, /docs/x/ redirects to /docs/x, and a miss is the 404
// page with status 404. For the browser tests and local audits; the real
// Worker is exercised by `npm run preview`.
//
//   node scripts/serve-dist.mjs [port] [dir]

import { createReadStream, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize, resolve } from "node:path";

const TYPES = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".webp": "image/webp",
  ".png": "image/png",
  ".woff2": "font/woff2",
  ".xml": "application/xml",
  ".txt": "text/plain; charset=utf-8",
};

function isFile(p) {
  try {
    return statSync(p).isFile();
  } catch {
    return false;
  }
}

export function startServer({ port = 0, dir = "dist/client" } = {}) {
  const root = resolve(dir);
  const server = createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");
    let path = decodeURIComponent(url.pathname);
    if (path.length > 1 && path.endsWith("/")) {
      res.writeHead(307, { location: path.replace(/\/+$/, "") + url.search });
      return res.end();
    }
    const base = normalize(join(root, path));
    if (!base.startsWith(root)) {
      res.writeHead(400);
      return res.end();
    }
    const candidates = [base, join(base, "index.html"), `${base}.html`];
    const file = candidates.find(isFile);
    const status = file ? 200 : 404;
    const served = file ?? join(root, "404", "index.html");
    res.writeHead(status, { "content-type": TYPES[extname(served)] ?? "application/octet-stream" });
    createReadStream(served).pipe(res);
  });
  return new Promise((ok) => server.listen(port, "127.0.0.1", () => ok(server)));
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const server = await startServer({ port: Number(process.argv[2] ?? 4321), dir: process.argv[3] });
  console.log(`serving dist/client on http://127.0.0.1:${server.address().port}`);
}
