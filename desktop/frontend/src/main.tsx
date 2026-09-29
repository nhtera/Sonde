// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// First: the server-mode token must be in place before the runtime calls.
import "./transport/server-auth";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles.css";

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
