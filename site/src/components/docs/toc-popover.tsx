// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { TOCPopover, type TOCPopoverProps } from "fumadocs-ui/layouts/docs/page/slots/toc";
import { strings } from "@/content/strings";

/**
 * The small-screen "On this page" popover. Fumadocs renders it as a <header>
 * outside <main>, which makes it a second page banner; inside an <aside> it
 * is scoped. `display: contents` keeps Fumadocs' grid placement.
 */
export function TocPopover(props: TOCPopoverProps) {
  return (
    <aside aria-label={strings.docs.tocMenu} style={{ display: "contents" }}>
      <TOCPopover {...props} />
    </aside>
  );
}
