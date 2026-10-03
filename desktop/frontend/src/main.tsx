// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// First: the server-mode token must be in place before the runtime calls.
import "./transport/server-auth";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./app/theme/fonts";
import "./app/theme/tokens.css";
// The other themes' tokens, after tokens.css: :root (Dark) must not win.
import "./app/theme/themes/index.css";
import "./app/shell.css";
import "./components/run/run.css";
import "./features/palette/palette.css";
// The shell's commands and panels, then each feature's (they register
// through app/registry; the shell never imports a feature's internals).
import "./app/core";
import "./features/editor";
import "./features/results";
import "./features/form";
import "./features/panels";
import "./features/import";
import "./features/copyas";
import "./features/perf";
import { App } from "./app/app";

// The test harness build (vite --mode harness) adds the spike and e2e
// hooks; other builds drop this branch.
if (import.meta.env.MODE === "harness") {
  void import("./harness").then((m) => m.start());
}

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}
