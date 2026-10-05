// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { CheckIcon, CopyIcon } from "@phosphor-icons/react/ssr";
import { useRef, useState, type ComponentProps } from "react";
import { strings } from "@/content/strings";

const t = strings.docs;

/**
 * A highlighted code block in the docs: Fumadocs' look, but the scrolling
 * area is a named, focusable group (Fumadocs makes it an unnamed region
 * landmark on every block) and the copy button uses the site's icons.
 */
export function CodeBlock({ children, className, ...props }: ComponentProps<"pre">) {
  const area = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    const text = area.current?.querySelector("pre")?.textContent ?? "";
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard denied: the text stays selectable.
    }
  };
  return (
    <figure dir="ltr" className={`shiki code-block not-prose ${className ?? ""}`} style={props.style}>
      <button type="button" className="code-copy" aria-live="polite" onClick={copy}>
        {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
        <span className="sr">{copied ? t.copied : t.copy}</span>
      </button>
      <div ref={area} role="group" aria-label={t.codeLabel} tabIndex={0} className="code-area fd-scroll-container">
        <pre {...props} className="min-w-full w-max *:flex *:flex-col">
          {children}
        </pre>
      </div>
    </figure>
  );
}
