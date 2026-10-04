// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import * as ContextMenu from "@radix-ui/react-context-menu";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { registry } from "../../app/registry";
import { confirm } from "../../components/ask";
import { CloseIcon } from "../../components/icons";
import { WorkspaceDesktop } from "../../lib/api";
import { isRequestPath } from "../../lib/files";
import { isMac, windowLook } from "../../lib/mode";
import { isDirty, useTabs } from "../../state/tabs";
import { copyPath, report } from "../tree/file-tree";

/** The open files; a dot marks unsaved edits. Right-click for the tab
 * menu (close others, close all…). */
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
          <ContextMenu.Root key={t.path}>
            <ContextMenu.Trigger asChild>
              <div
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
            </ContextMenu.Trigger>
            <TabMenu path={t.path} />
          </ContextMenu.Root>
        );
      })}
    </div>
  );
}

/** The tab's menu: its file's actions, then the closing ones, each the
 * command of the same id (the palette and the keys run them too). */
function TabMenu({ path }: { path: string }) {
  const tabs = useTabs((s) => s.tabs);
  const keys = {
    run: useKeyLabel("file.run"),
    save: useKeyLabel("file.save"),
    close: useKeyLabel("tab.close"),
    force: useKeyLabel("tab.closeWithoutSaving"),
  };
  const at = tabs.findIndex((t) => t.path === path);
  const tab = tabs[at];
  if (!tab) return null;
  const item = (text: string, run: () => unknown, hint?: string, disabled = false) => (
    <ContextMenu.Item className="menu-item" disabled={disabled} onSelect={() => void Promise.resolve(run()).catch(report)}>
      {text}
      {hint && <span className="hint">{hint}</span>}
    </ContextMenu.Item>
  );
  const cmd = (id: string) => () => registry.getCommand(id)?.run(path);
  return (
    <ContextMenu.Portal>
      <ContextMenu.Content className="menu">
        <ContextMenu.Label className="menu-label mono">{path}</ContextMenu.Label>
        {isRequestPath(path) && item("Run file", () => runTab(path), keys.run)}
        {item("Save", () => useTabs.getState().save(path), keys.save, !isDirty(tab))}
        <ContextMenu.Separator className="menu-sep" />
        {item("Close tab", cmd("tab.close"), keys.close)}
        {item("Close without saving", cmd("tab.closeWithoutSaving"), keys.force, !isDirty(tab))}
        {item("Close other tabs", cmd("tab.closeOthers"), undefined, tabs.length < 2)}
        {item("Close tabs to the right", cmd("tab.closeRight"), undefined, at === tabs.length - 1)}
        {item("Close saved tabs", cmd("tab.closeSaved"), undefined, !tabs.some((t) => !isDirty(t)))}
        {item("Close all tabs", cmd("tab.closeAll"))}
        {item("Close all without saving", cmd("tab.closeAllWithoutSaving"), undefined, !tabs.some(isDirty))}
        <ContextMenu.Separator className="menu-sep" />
        {item("Copy path", () => copyPath(path))}
        {windowLook && item(isMac ? "Reveal in Finder" : "Reveal in file manager", () => WorkspaceDesktop.Reveal(path))}
      </ContextMenu.Content>
    </ContextMenu.Portal>
  );
}

async function runTab(path: string) {
  useTabs.getState().activate(path);
  await registry.getCommand("file.run")?.run();
}

/** Closes a tab, asking first when it has unsaved edits. */
export function closeTab(path: string) {
  return closeTabs([path]);
}

/** Closes tabs. With unsaved edits among them, asks once first (unless
 * force): canceling keeps every tab open. */
export async function closeTabs(paths: string[], force = false) {
  const set = new Set(paths);
  const dirty = useTabs.getState().tabs.filter((t) => set.has(t.path) && isDirty(t));
  if (!force && dirty.length > 0) {
    const message =
      dirty.length === 1 && paths.length === 1
        ? `Close ${dirty[0].path} without saving?`
        : `${dirty.length === 1 ? "1 file has" : `${dirty.length} files have`} unsaved changes: ${dirty.map((t) => t.path).join(", ")}. Close without saving?`;
    if (!(await confirm({ title: "Unsaved changes", message, submit: "Close without saving" }))) return;
  }
  useTabs.getState().closeMany(paths);
}
