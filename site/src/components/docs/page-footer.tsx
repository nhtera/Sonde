// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { PencilSimpleIcon } from "@phosphor-icons/react/ssr";
import { strings } from "@/content/strings";
import { editUrl } from "@/lib/doc-paths";

/** Edit on GitHub (from the validated source path) and the last commit date of the doc. */
export function PageFooter({ source, lastUpdated }: { source: string; lastUpdated?: string }) {
  return (
    <div className="docs-meta">
      <a href={editUrl(source)} rel="noreferrer noopener" target="_blank">
        <PencilSimpleIcon aria-hidden="true" />
        {strings.docs.edit}
      </a>
      {lastUpdated ? (
        <span>
          {strings.docs.lastUpdated} <time dateTime={lastUpdated}>{lastUpdated}</time>
        </span>
      ) : null}
    </div>
  );
}
