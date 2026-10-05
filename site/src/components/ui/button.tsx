// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { AnchorHTMLAttributes, ReactNode } from "react";

type Props = AnchorHTMLAttributes<HTMLAnchorElement> & {
  variant?: "primary" | "ghost";
  children: ReactNode;
};

/** A link styled as a button: primary (teal) or ghost (outlined). */
export function ButtonLink({ variant = "ghost", className, children, ...rest }: Props) {
  return (
    <a className={["btn", variant === "primary" ? "primary" : "", className ?? ""].filter(Boolean).join(" ")} {...rest}>
      {children}
    </a>
  );
}
