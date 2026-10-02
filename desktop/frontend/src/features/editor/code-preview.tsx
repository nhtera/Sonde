// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { useEffect, useRef } from "react";
import { sondeLanguage } from "../../lang";
import { methodColors } from "./methods";
import { editorHighlight, editorTheme } from "./theme";

/** Read-only file text, colored as the editor colors it (an import's
 * preview). */
export function CodePreview({ text, label, className }: { text: string; label: string; className?: string }) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: text,
        extensions: [
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          sondeLanguage(),
          editorTheme,
          editorHighlight,
          methodColors,
        ],
      }),
    });
    return () => v.destroy();
  }, [text, label]);
  return <div ref={host} className={`code-preview ${className ?? ""}`} role="region" aria-label={label} />;
}
