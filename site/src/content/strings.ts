// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Every user-facing string of the site chrome and the landing page. English
// only for now; a second language adds a sibling module, not edits to
// components.
//
// Code samples are token lists: a plain string is uncolored text, a pair is
// [class, text] where the class is a syntax or signal class from
// components.css (t-var, m-post, ok, bad…).
//
// Naming rule: "Hurl" appears only as the file-format name.

export type Token = string | readonly [string, string];
export type Line = readonly Token[];

export const strings = {
  site: {
    name: "Sonde",
    title: "Sonde: plain-text HTTP tests for humans, CI and AI agents",
    description:
      "Write requests and asserts in .hurl files. Run them in Sonde Desktop, with sonde --test in CI, or through sonde mcp for your agents.",
    domain: "sonde.erai.dev",
  },
  theme: {
    toLight: "Switch to light theme",
    toDark: "Switch to dark theme",
  },
  docs: {
    titleSuffix: " · Sonde docs",
    edit: "Edit on GitHub",
    lastUpdated: "Last updated",
    toc: "On this page",
    tocMenu: "Page sections",
    codeLabel: "Code sample",
    tableLabel: "Table",
    copy: "Copy code",
    copied: "Copied",
  },
  notFound: {
    title: "Page not found",
    body: "Nothing lives at this address.",
    home: "Back to the home page",
    docs: "Read the docs",
    metaTitle: "Page not found · Sonde",
  },
  nav: {
    skip: "Skip to content",
    home: "Sonde home",
    main: "Main",
    mobile: "Mobile",
    docs: "Docs",
    desktop: "Desktop",
    github: "GitHub",
    search: "Search docs",
    openMenu: "Open menu",
    closeMenu: "Close menu",
  },
  hero: {
    eyebrow: "Open source CLI and desktop app",
    title: "Plain-text HTTP tests for humans, CI and AI agents.",
    // 20 words: "Requests and asserts in .hurl files. Run them in Sonde Desktop,
    // with `sonde --test` in CI, or through `sonde mcp`."
    sub: [
      "Requests and asserts in .hurl files. Run them in Sonde Desktop, with ",
      ["code", "sonde --test"],
      " in CI, or through ",
      ["code", "sonde mcp"],
      ".",
    ] as Line,
    installLabel: "Install with",
    commandLabel: "Install command",
    copy: "Copy",
    copied: "Copied",
    copyFailed: "Select and copy",
    download: "Download Sonde Desktop",
    readDocs: "Read the docs",
    shotAlt: "Sonde Desktop running checkout.hurl: four requests pass, the fifth fails because the order status is pending instead of paid.",
    shotAltLight: "The same run in the light theme.",
  },
  three: {
    title: "One file. Three places it runs.",
    body: "The desktop app, the CLI and the MCP server share one engine. A file that passes on your machine passes in CI and for your agent.",
    file: "orders/checkout.hurl",
    fileBadge: ".hurl · Hurl 8",
    fileLabel: "checkout.hurl source",
    checkout: [
      { n: 24, mark: "request", line: [["m-post", "POST"], " ", ["t-var", "{{base_url}}"], "/orders"] },
      { n: 25, line: [["t-key", "Authorization"], ": Bearer ", ["t-var", "{{token}}"]] },
      { n: 26, line: [["t-key", "Content-Type"], ": application/json"] },
      { n: 27, line: ['{"cart_id": "', ["t-var", "{{cart_id}}"], '"}'] },
      { n: 28, mark: "pass", line: ["HTTP ", ["t-num", "201"]] },
      { n: 29, line: [["t-sec", "[Captures]"]] },
      { n: 30, mark: "capture", line: ["order_id: ", ["t-f", "jsonpath"], " ", ["t-str", '"$.order_id"']] },
      { n: 31, line: [] },
      { n: 32, mark: "request", line: [["m-get", "GET"], " ", ["t-var", "{{base_url}}"], "/orders/", ["t-var", "{{order_id}}"]] },
      { n: 33, line: [["t-key", "Authorization"], ": Bearer ", ["t-var", "{{token}}"]] },
      { n: 34, mark: "pass", line: ["HTTP ", ["t-num", "200"]] },
      { n: 35, line: [["t-sec", "[Asserts]"]] },
      { n: 36, mark: "fail", failed: true, error: 'got "pending"', line: [["t-f", "jsonpath"], " ", ["t-str", '"$.status"'], " == ", ["t-str", '"paid"']] },
      { n: 37, mark: "pass", line: [["t-f", "jsonpath"], " ", ["t-str", '"$.total"'], " == ", ["t-num", "25.8"]] },
    ],
    desktop: {
      name: "Sonde Desktop",
      action: "run file",
      kbd: "⌘R",
      where: "local",
      rows: [
        { i: "4", method: "POST", path: "/orders", chips: [{ name: "order_id", dir: "out" }], status: "201", ok: true },
        { i: "5", method: "GET", path: "/orders/ord_1093", chips: [], status: "200", ok: false },
      ],
    },
    ci: {
      name: "CI",
      action: "sonde --test",
      where: "GitHub Actions",
      label: "CI output",
      output: [
        [["bad", "error"], ": Assert failure"],
        ["  --> orders/checkout.hurl:36:0"],
        ["   |"],
        ["   | GET {{base_url}}/orders/{{order_id}}"],
        ["   | ..."],
        ['36 | jsonpath "$.status" == "paid"'],
        ["   |   actual:   string <pending>"],
        ["   |   expected: string <paid>"],
        [],
        [["bad", "Failure"], " orders/checkout.hurl (5 request(s) in 116 ms)"],
      ] as Line[],
    },
    agent: {
      name: "AI agent",
      action: "sonde_run",
      where: "sonde mcp --allow-run",
      label: "Agent tool result",
      output: [
        [["t-com", "// tool result"]],
        ["{ ", ["t-key", '"filename"'], ": ", ["t-str", '"orders/checkout.hurl"'], ","],
        ["  ", ["t-key", '"success"'], ": ", ["t-num", "false"], ", ", ["t-key", '"time"'], ": ", ["t-num", "116"], ","],
        ["  ", ["t-key", '"entries"'], ": [ ", ["t-com", "… 5 entries, 1 failed assert …"], " ] }"],
      ] as Line[],
    },
  },
  loud: {
    title: "Calm surfaces.",
    titleLoud: "Loud results.",
    body: "The interface stays quiet so the run can speak. Color appears only when it means something: a pass, a failure, a captured value, the method of a request.",
    figureLabel: "Run result: 8 passed, 1 failed. Assert failed at line 36, expected paid, actual pending.",
    file: "checkout.hurl",
    failed: "Failed",
    where: "local",
    counts: { passed: "8 passed", failed: "1 failed", time: "116 ms", ago: "12s ago" },
    passed: "passed",
    failedLabel: "failed",
    rows: [
      { i: "1", method: "POST", path: "/auth/login", chips: [{ name: "token", dir: "out" }], status: "200", ms: "29 ms", ok: true },
      { i: "2", method: "POST", path: "/carts", chips: [{ name: "token", dir: "in" }, { name: "cart_id", dir: "out" }], status: "201", ms: "29 ms", ok: true },
      { i: "3", method: "POST", path: "/carts/c_8f2a41/items", chips: [], status: "200", ms: "20 ms", ok: true },
      { i: "4", method: "POST", path: "/orders", chips: [{ name: "order_id", dir: "out" }], status: "201", ms: "10 ms", ok: true },
      { i: "5", method: "GET", path: "/orders/ord_1093", chips: [{ name: "order_id", dir: "in" }], status: "200", ms: "28 ms", ok: false },
    ],
    assertTitle: "Assert failed",
    assertLine: "line 36",
    assertExpr: [["t-f", "jsonpath"], " ", ["t-str", '"$.status"'], " == ", ["t-str", '"paid"']] as Line,
    expected: "Expected",
    actual: "Actual",
    expectedValue: '"paid"',
    actualValue: '"pending"',
    goTo: "Go to line 36",
    goToKbd: "⌘G",
    passedAsserts: [
      { ln: "L34", text: "HTTP 200" },
      { ln: "L37", text: 'jsonpath "$.total" == 25.8' },
    ],
  },
  features: {
    title: "Everything an API test needs. In text.",
    body: "Each feature is a few lines in a file you can review in a pull request.",
    git: {
      title: "Plain text, in git",
      body: "Requests, captures and asserts live in .hurl files next to your code. Review them, diff them, blame them.",
      more: "File format",
      diff: [
        [" ", ["t-com", "orders/checkout.hurl"]],
        [" ", ["t-sec", "[Asserts]"]],
        [["del", '- jsonpath "$.status" == "created"']],
        [["add", '+ jsonpath "$.status" == "paid"']],
        [["add", '+ jsonpath "$.total" == 25.8']],
      ] as Line[],
    },
    env: {
      title: "Environments and secrets",
      body: "One sonde.yaml per project. Secrets stay in files git ignores and print as stars.",
      more: "sonde.yaml",
      shotLabel: "Variable autocomplete listing token from a capture, base_url from sonde.yaml and api_key from a secrets file, shown as stars.",
    },
    oas: {
      title: "OpenAPI contracts",
      body: "Check every response against your spec and see which operations your tests cover.",
      more: "OpenAPI guide",
      shotLabel: "Contract coverage: operations from openapi.yaml exercised by the tests, listed by method and path.",
    },
    data: {
      title: "Data-driven runs",
      body: "Run a file once per row of a CSV or JSON file.",
      more: "Data-driven guide",
      rows: [
        ["row 1  ada@example.com    ", ["ok", "✓"]],
        ["row 2  grace@example.com  ", ["ok", "✓"]],
        ["row 3  not-an-email       ", ["bad", "✕ 422"]],
      ] as Line[],
    },
    stream: {
      title: "Streams and gRPC",
      body: "Server-Sent Events, WebSocket and gRPC calls in .sonde files.",
      more: "Streaming guide",
      log: [
        [["t-com", "event"], " order.created  ", ["muted", "0.2s"]],
        [["t-com", "event"], " order.paid     ", ["muted", "1.4s"]],
        [["ok", "until matched"], " · closed"],
      ] as Line[],
    },
    imports: {
      title: "Imports that write plain text",
      body: "Bring requests from curl, .http files, OpenAPI specs and collection exports. Secrets are lifted into variables.",
      more: "Import guide",
      sample: [
        [["muted", "$"], " sonde import curl < login.sh"],
        [["m-post", "POST"], " ", ["t-var", "{{base_url}}"], "/auth/login"],
        [["t-key", "Authorization"], ": Bearer ", ["t-var", "{{api_token}}"]],
      ] as Line[],
    },
    lsp: {
      title: "Your editor knows the format",
      body: "sonde lsp brings diagnostics, completion and hover to VS Code, Neovim and any LSP client.",
      more: "Editor setup",
      typed: ["X-Client: ", ["t-var", "{{na"]] as Line,
      completions: [
        { name: "name", detail: "sonde.yaml · local", on: true },
        { name: "namespace", detail: "capture · line 6", on: false },
      ],
    },
    reports: {
      title: "Reports your pipeline already reads",
      body: "One run, the formats CI dashboards and agents expect.",
      formats: ["--report-json", "--report-junit", "--report-tap", "--report-html"],
    },
  },
  showcase: {
    title: "Sonde Desktop. The same files, in a window.",
    body: "A free app for macOS, Windows and Linux that reads your project folder. Nothing to sign in to.",
    tabsLabel: "Desktop screens",
    tabs: [
      { id: "run", title: "Test run", sub: "Every file, one summary", alt: "Test run of the project's files with parallel jobs: passes and failures, with export to HTML, JSON, JUnit and TAP." },
      { id: "palette", title: "Command palette", sub: "Files and commands, ⌘K", alt: "Command palette open over the editor, listing request files and commands such as Check file and Run to cursor." },
      { id: "form", title: "Form view", sub: "Edit visually, writes text", alt: "Form view of the Asserts tab with a failing row explained and the text it writes." },
      { id: "streams", title: "Streams", sub: "Live Server-Sent Events", alt: "A live Server-Sent Events log that closes when the until condition matches." },
      { id: "agents", title: "AI agents", sub: "MCP config and allowlist", alt: "AI agents panel with the sonde mcp configuration, allow-run switch and host allowlist." },
    ],
    download: "Download Sonde Desktop",
    note: "macOS, Windows and Linux. Signed, with checksums.",
  },
  privacy: {
    title: "Your requests stay yours.",
    items: [
      { title: "No account", body: "Open a folder and start. There is nothing to sign in to." },
      { title: "No telemetry", body: "Sonde sends the requests you wrote. Nothing about you." },
      { title: "Local history", body: "Runs are kept on your computer, with secrets redacted." },
      { title: "Your call on updates", body: "The daily update check can be turned off." },
    ],
  },
  quickstart: {
    title: "Your first test in a minute.",
    steps: [
      { title: "Install", body: "One binary from Homebrew, Scoop, Go, or a signed archive." },
      { title: "Write a file", body: "A request, then the response you expect." },
      { title: "Run it", body: "The same command on your laptop and in CI." },
    ],
    terminalLabel: "Terminal session",
    // Real output format of `sonde --test`; timings are illustrative.
    terminal: [
      [["muted", "$"], " brew install nhtera/tap/sonde"],
      [],
      [["muted", "$"], " cat health.hurl"],
      [["m-get", "GET"], " https://api.example.com/health"],
      ["HTTP ", ["t-num", "200"]],
      [],
      [["muted", "$"], " sonde --test health.hurl"],
      [["ok", "Success"], " health.hurl (1 request(s) in 41 ms)"],
      ["--------------------------------------------------------"],
      ["Executed files:    1"],
      ["Executed requests: 1 (24.4/s)"],
      ["Succeeded files:   1 (100.0%)"],
      ["Failed files:      0 (0.0%)"],
      ["Duration:          41 ms (0h:0m:0s:41ms)"],
    ] as Line[],
  },
  footer: {
    tagline: "Plain-text HTTP tests for humans, CI and AI agents.",
    star: "Star on GitHub",
    docs: { title: "Docs", links: { start: "Getting started", desktop: "Sonde Desktop", guides: "Guides", cli: "CLI reference" } },
    project: { title: "Project", links: { releases: "Releases", changelog: "Changelog", contributing: "Contributing" } },
    trust: { title: "Trust", links: { security: "Security", stability: "Stability promise", trademarks: "Trademarks" } },
    legal: "Apache-2.0 · © 2026 The Sonde Authors",
  },
} as const;
