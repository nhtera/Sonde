// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The form's text field: it edits a local copy and commits on Enter or
// when it loses the focus (one edit of the file, not one per keystroke),
// and offers completions: {{variables}} in values (the same list as the
// Text view), header names in keys.

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Vars, type ScopeVar } from "../../lib/api";
import { useEnv } from "../../state/env";

export interface Suggestion {
  label: string;
  /** What replaces the typed part. */
  insert: string;
  detail?: string;
  /** Muted text on the right (a variable's source). */
  source?: string;
}

/** Completions for the text before the caret: the items and where the
 * part they replace starts (null: none). */
export type Suggester = (before: string) => Promise<{ from: number; items: Suggestion[] } | null>;

let vars: { key: string; list: Promise<ScopeVar[]> } | null = null;

/** {{variable}} completions of file (the variables its run would see). */
export function varSuggester(file: string): Suggester {
  return async (before) => {
    const m = /\{\{([\w.-]*)$/.exec(before);
    if (!m) return null;
    const env = useEnv.getState().current;
    const key = `${file}|${env}`;
    // Asked again each time a completion opens: captures change per run.
    if (vars?.key !== key || m[1] === "") vars = { key, list: Vars.For(file, env).then((l) => l ?? [], () => []) };
    const list = await vars.list;
    const items = list
      .filter((v) => v.name.toLowerCase().startsWith(m[1].toLowerCase()))
      .slice(0, 8)
      .map((v) => ({ label: v.name, insert: `{{${v.name}}}`, detail: v.secret ? "***" : v.display, source: v.origin || v.source }));
    return { from: before.length - m[0].length, items };
  };
}

/** Completions from a fixed list, for the whole field (header names). */
export function listSuggester(list: { name: string; hint?: string }[]): Suggester {
  return async (before) => {
    if (!before || before.includes("{{")) return null;
    const items = list
      .filter((h) => h.name.toLowerCase().startsWith(before.toLowerCase()) && h.name.toLowerCase() !== before.toLowerCase())
      .slice(0, 6)
      .map((h) => ({ label: h.name, insert: h.name, detail: h.hint }));
    return { from: 0, items };
  };
}

export interface SuggestInputProps {
  value: string;
  onCommit(value: string): void;
  suggest?: Suggester;
  placeholder?: string;
  label: string;
  className?: string;
  disabled?: boolean;
  readOnly?: boolean;
}

export function SuggestInput({ value, onCommit, suggest, placeholder, label, className, disabled, readOnly }: SuggestInputProps) {
  // The text being edited; the file's value otherwise.
  const [draft, setDraft] = useState<string | null>(null);
  const text = draft ?? value;
  const [open, setOpen] = useState<{ from: number; items: Suggestion[] } | null>(null);
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const asked = useRef(0);
  // A field removed while typed in (⌘E, another request) keeps what was
  // typed: it commits on its way out.
  const latest = useRef({ draft, value, onCommit });
  useEffect(() => {
    latest.current = { draft, value, onCommit };
  });
  useEffect(
    () => () => {
      const { draft: d, value: v, onCommit: c } = latest.current;
      if (d !== null && d !== v) c(d);
    },
    [],
  );

  const ask = async (t: string, caret: number) => {
    if (!suggest) return;
    const n = ++asked.current;
    const res = await suggest(t.slice(0, caret));
    if (n !== asked.current) return;
    setOpen(res && res.items.length ? res : null);
    setActive(0);
  };
  const commit = (t = text) => {
    setOpen(null);
    setDraft(null);
    if (t !== value) onCommit(t);
  };
  const pick = (s: Suggestion) => {
    const caret = input.current?.selectionStart ?? text.length;
    const next = text.slice(0, open!.from) + s.insert + text.slice(caret);
    setDraft(next);
    setOpen(null);
    requestAnimationFrame(() => {
      const at = open!.from + s.insert.length;
      input.current?.setSelectionRange(at, at);
    });
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (open) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setActive((a) => (a + (e.key === "ArrowDown" ? 1 : -1) + open.items.length) % open.items.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        pick(open.items[active]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        setOpen(null);
        return;
      }
    }
    if (e.key === "Enter") {
      e.preventDefault();
      commit();
    }
    if (e.key === "Escape") {
      setDraft(null);
      setOpen(null);
    }
  };

  return (
    <span className="suggest">
      <input
        ref={input}
        className={className}
        aria-label={label}
        value={text}
        placeholder={placeholder}
        disabled={disabled}
        readOnly={readOnly}
        spellCheck={false}
        onBlur={() => draft !== null && commit(draft)}
        onChange={(e) => {
          setDraft(e.target.value);
          void ask(e.target.value, e.target.selectionStart ?? e.target.value.length);
        }}
        onKeyDown={onKey}
      />
      {open && (
        <ul className="suggest-list" role="listbox" aria-label={`${label} completions`}>
          {open.items.map((s, i) => (
            <li
              key={s.label}
              role="option"
              aria-selected={i === active}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(s);
              }}
            >
              <span className="mono">{s.label}</span>
              {s.source && <span className="src">{s.source}</span>}
              {s.detail && <span className="detail mono">{s.detail}</span>}
            </li>
          ))}
        </ul>
      )}
    </span>
  );
}
