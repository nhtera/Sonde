// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";

/** A key chip such as ⌘K. Decorative by default: the action it names is labeled elsewhere. */
export function Kbd({ children, label }: { children: ReactNode; label?: string }) {
  return label ? (
    <kbd className="kbd" aria-label={label}>
      {children}
    </kbd>
  ) : (
    <span className="kbd" aria-hidden="true">
      {children}
    </span>
  );
}
