// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { confirm } from "../../components/ask";
import { CloseIcon } from "../../components/icons";
import { isDirty, useTabs } from "../../state/tabs";

/** The open files; a dot marks unsaved edits. */
export function TabsBar() {
  const tabs = useTabs((s) => s.tabs);
  const active = useTabs((s) => s.active);
  if (tabs.length === 0) return null;
  return (
    <div className="tabs" role="tablist" aria-label="Open files">
      {tabs.map((t) => {
        const name = t.path.split("/").at(-1);
        const dirty = isDirty(t);
        return (
          <div
            key={t.path}
            role="tab"
            tabIndex={0}
            className="tab"
            title={t.path}
            aria-selected={active === t.path}
            onClick={() => useTabs.getState().activate(t.path)}
            onKeyDown={(e) => {
              if (e.key === "Enter") useTabs.getState().activate(t.path);
              else if (e.key === "Delete" || e.key === "Backspace") void closeTab(t.path);
            }}
            onAuxClick={(e) => e.button === 1 && void closeTab(t.path)}
          >
            <i className="ic" style={{ background: t.path.endsWith(".sonde") ? "var(--m-patch)" : "var(--pass)" }} />
            {name}
            {dirty && <i className="dirty" aria-label="unsaved" />}
            {/* Out of the tab order: Delete closes the focused tab. */}
            <button
              className="icon-btn x"
              tabIndex={-1}
              aria-label={`Close ${name}`}
              onClick={(e) => {
                e.stopPropagation();
                void closeTab(t.path);
              }}
            >
              <CloseIcon />
            </button>
          </div>
        );
      })}
    </div>
  );
}

/** Closes a tab, asking first when it has unsaved edits. */
export async function closeTab(path: string) {
  const t = useTabs.getState().tabs.find((x) => x.path === path);
  if (t && isDirty(t) && !(await confirm({ title: "Unsaved changes", message: `Close ${path} without saving?`, submit: "Close without saving" }))) return;
  useTabs.getState().close(path);
}
