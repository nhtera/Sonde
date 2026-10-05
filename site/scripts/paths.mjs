// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { fileURLToPath } from "node:url";

/** site/ */
export const SITE_DIR = fileURLToPath(new URL("..", import.meta.url));

/**
 * The static assets the build writes and the deploy uploads (Cloudflare
 * Build Output). Every check runs on this directory.
 */
export const OUTPUT_DIR = fileURLToPath(new URL("../.cloudflare/output/v0/workers/default/assets", import.meta.url));
