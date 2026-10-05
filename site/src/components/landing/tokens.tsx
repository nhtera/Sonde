// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Fragment } from "react";
import type { Line, Token } from "@/content/strings";

/** One token of a code sample: plain text, or [class, text] (`code` renders a <code>). */
export function Tok({ t }: { t: Token }) {
  if (typeof t === "string") return <>{t}</>;
  const [cls, text] = t;
  return cls === "code" ? <code className="mono">{text}</code> : <span className={cls}>{text}</span>;
}

/** A line of tokens. */
export function Toks({ line }: { line: Line }) {
  return (
    <>
      {line.map((t, i) => (
        <Tok key={i} t={t} />
      ))}
    </>
  );
}

/** Lines of tokens joined by newlines, for a <pre>. */
export function Lines({ lines }: { lines: readonly Line[] }) {
  return (
    <>
      {lines.map((line, i) => (
        <Fragment key={i}>
          {i > 0 ? "\n" : null}
          <Toks line={line} />
        </Fragment>
      ))}
    </>
  );
}
