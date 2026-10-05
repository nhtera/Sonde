// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Self-hosted fonts (OFL-1.1), the desktop app's: Geist for the UI,
// JetBrains Mono for anything the engine reads or writes. No remote fonts.
// The @font-face rules come from global.css; these two (latin, the body and
// the headline weight) are preloaded so first paint uses them.
import geist400 from "@fontsource/geist/files/geist-latin-400-normal.woff2?url";
import geist600 from "@fontsource/geist/files/geist-latin-600-normal.woff2?url";

export const fontPreloads = [geist400, geist600].map((href) => ({
  rel: "preload",
  href,
  as: "font",
  type: "font/woff2",
  crossOrigin: "anonymous" as const,
}));
