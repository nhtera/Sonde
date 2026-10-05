// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { createFromSource } from "fumadocs-core/search/server";
import { source } from "@/lib/source";

const server = createFromSource(source, { language: "english" });

// Prerendered to a static JSON file; the search dialog downloads it.
export const Route = createFileRoute("/api/search.json")({
  server: {
    handlers: {
      GET: () => server.staticGET(),
    },
  },
});
