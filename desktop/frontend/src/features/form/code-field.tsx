// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A small code editor for a body (JSON, XML, text, a GraphQL query, a
// gRPC message): highlighted, committed when typing pauses or it loses
// the focus. The file's text replaces it while it is not being edited.

import { history, historyKeymap, defaultKeymap } from "@codemirror/commands";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import { useEffect, useRef } from "react";
import { ownKeys } from "../editor/keymap";
import { editorHighlight, editorTheme } from "../editor/theme";

/** No language (one value: the editor is made once). */
const plain: Extension = [];

export interface CodeFieldProps {
  value: string;
  onCommit(value: string): void;
  language?: Extension;
  label: string;
  minLines?: number;
}

export function CodeField({ value, onCommit, language = plain, label, minLines = 4 }: CodeFieldProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const commit = useRef(onCommit);
  const last = useRef(value);
  useEffect(() => {
    commit.current = onCommit;
  });

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const flush = () => {
      clearTimeout(timer);
      const text = view.current?.state.doc.toString() ?? "";
      if (text !== last.current) {
        last.current = text;
        commit.current(text);
      }
    };
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        extensions: [
          // The app's keys (⌘↵ sends) are not the editor's.
          ownKeys,
          lineNumbers(),
          history(),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          language,
          editorTheme,
          editorHighlight,
          EditorView.contentAttributes.of({ "aria-label": label }),
          EditorView.updateListener.of((u) => {
            if (!u.docChanged) return;
            clearTimeout(timer);
            timer = setTimeout(flush, 800);
          }),
          EditorView.domEventHandlers({ blur: () => flush() }),
        ],
      }),
    });
    view.current = v;
    return () => {
      flush();
      v.destroy();
      view.current = null;
    };
    // The editor is made once; value changes are applied below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, label]);

  // The file's text, unless the field is being edited.
  useEffect(() => {
    const v = view.current;
    if (!v || v.hasFocus || v.state.doc.toString() === value) return;
    last.current = value;
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
  }, [value]);

  return <div className="code-field" ref={host} style={{ minHeight: `${minLines * 20 + 12}px` }} />;
}
