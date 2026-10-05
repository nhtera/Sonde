// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { HTMLAttributes, ReactNode } from "react";

/** The rounded window an app screenshot sits in (hairline, overlay shadow). */
export function WindowFrame({ children, className, ...rest }: HTMLAttributes<HTMLDivElement> & { children: ReactNode }) {
  return (
    <div className={["frame", className ?? ""].filter(Boolean).join(" ")} {...rest}>
      {children}
    </div>
  );
}
