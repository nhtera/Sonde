// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Every user-facing string of the site chrome and the landing page. English
// only for now; a second language adds a sibling module, not edits to
// components.

export const strings = {
  site: {
    name: "Sonde",
    title: "Sonde: plain-text HTTP tests for humans, CI and AI agents",
    description:
      "Write requests and asserts in .hurl files. Run them in Sonde Desktop, with sonde --test in CI, or through sonde mcp for your agents.",
  },
  theme: {
    toLight: "Switch to light theme",
    toDark: "Switch to dark theme",
  },
  docs: {
    titleSuffix: " · Sonde docs",
    edit: "Edit on GitHub",
    lastUpdated: "Last updated",
  },
  notFound: {
    title: "Page not found",
    body: "Nothing lives at this address.",
    home: "Back to the home page",
  },
} as const;
