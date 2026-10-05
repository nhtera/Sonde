// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The antenna mark (editors/vscode/images/icon.svg). The one SVG drawn by
// hand on the site, and the only place hex colors appear: the mark keeps
// its colors in both themes.
export function Logo({ size = 26 }: { size?: number }) {
  return (
    <svg viewBox="0 0 128 128" width={size} height={size} aria-hidden="true" focusable="false">
      <rect width="128" height="128" rx="24" fill="#121A2B" />
      <g fill="none" strokeWidth="6.5" strokeLinecap="round">
        <path d="M36 62a30 30 0 0 1 30 30" stroke="#2F5C58" />
        <path d="M36 44a48 48 0 0 1 48 48" stroke="#3E7F78" />
        <path d="M36 26a66 66 0 0 1 66 66" stroke="#5DC9B8" />
      </g>
      <circle cx="36" cy="92" r="11" fill="#5DC9B8" />
    </svg>
  );
}
