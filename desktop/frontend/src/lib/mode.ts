// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { isServerMode } from "../transport/server-auth";

/**
 * Whether the page is served over HTTP (server mode, or the test
 * harness) rather than in the desktop window: the window-only features
 * (folder dialogs, reveal, trash, copy with secret values) are hidden.
 */
export const serverMode = isServerMode(window.location);

/** The platform, for key labels and the title bar. */
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
