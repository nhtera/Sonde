// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute, Link } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  component: Home,
});

// Placeholder until the landing page (phase 4).
function Home() {
  return (
    <main id="main">
      <h1>Sonde</h1>
      <Link to="/docs/$" params={{ _splat: "" }}>
        Docs
      </Link>
    </main>
  );
}
