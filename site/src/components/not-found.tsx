// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "@tanstack/react-router";
import { strings } from "@/content/strings";

export function NotFound() {
  return (
    <main id="main" className="not-found">
      <h1>{strings.notFound.title}</h1>
      <p>{strings.notFound.body}</p>
      <Link to="/">{strings.notFound.home}</Link>
    </main>
  );
}
