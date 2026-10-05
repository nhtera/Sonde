// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useRef, useState, type KeyboardEvent } from "react";
import install from "../../../content/generated/install.json";
import { strings } from "@/content/strings";

const t = strings.hero;

/**
 * Install commands, read from README.md ## Install at build time. One line
 * per command: a long one scrolls sideways (focusable) with a fade edge.
 * Tabs use a roving tabindex with arrow keys.
 */
export function InstallTabs() {
  const [current, setCurrent] = useState(0);
  const [copy, setCopy] = useState<"idle" | "done" | "failed">("idle");
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  const pick = (i: number, focus = false) => {
    setCurrent(i);
    setCopy("idle");
    if (focus) tabs.current[i]?.focus();
  };
  const onKey = (e: KeyboardEvent, i: number) => {
    const d = ({ ArrowRight: 1, ArrowLeft: -1 } as Record<string, number>)[e.key];
    if (!d) return;
    e.preventDefault();
    pick((i + d + install.length) % install.length, true);
  };
  const command = install[current].command;
  return (
    <>
      <div className="tabs" role="tablist" aria-label={t.installLabel}>
        {install.map((tab, i) => (
          <button
            key={tab.id}
            ref={(el) => {
              tabs.current[i] = el;
            }}
            role="tab"
            id={`it-${tab.id}`}
            aria-selected={i === current}
            aria-controls="install-cmd"
            tabIndex={i === current ? 0 : -1}
            onClick={() => pick(i)}
            onKeyDown={(e) => onKey(e, i)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div className="cmd" id="install-cmd" role="tabpanel" aria-labelledby={`it-${install[current].id}`}>
        <span className="p" aria-hidden="true">
          $
        </span>
        <pre tabIndex={0} aria-label={t.commandLabel}>
          {command}
        </pre>
        <button
          className={copy === "done" ? "copy done" : "copy"}
          type="button"
          aria-live="polite"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(command);
              setCopy("done");
            } catch {
              setCopy("failed");
            }
          }}
        >
          {copy === "done" ? t.copied : copy === "failed" ? t.copyFailed : t.copy}
        </button>
      </div>
    </>
  );
}
