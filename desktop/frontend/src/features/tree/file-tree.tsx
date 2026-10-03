// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import * as ContextMenu from "@radix-ui/react-context-menu";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, useState } from "react";
import { fromEvent, label, normalize } from "../../app/keymap/keymap-manager";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { registry } from "../../app/registry";
import { ask, confirm } from "../../components/ask";
import { ChevronDown, ImportIcon, LockIcon, PlusIcon, SearchIcon } from "../../components/icons";
import { Clipboard } from "@wailsio/runtime";
import { Workspace, WorkspaceDesktop, appError } from "../../lib/api";
import { isMac, serverMode, windowLook } from "../../lib/mode";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";
import { byFile, reqLabel, rows as flatten, type Row } from "./rows";

const kindColor: Record<string, string> = {
  request: "var(--info)",
  config: "var(--warn)",
  secrets: "var(--warn)",
  data: "var(--info)",
  file: "var(--faint)",
};

function Highlight({ text, hl }: { text: string; hl?: [number, number] }) {
  if (!hl) return <>{text}</>;
  return (
    <>
      {text.slice(0, hl[0])}
      <b className="hl">{text.slice(hl[0], hl[1])}</b>
      {text.slice(hl[1])}
    </>
  );
}

export function FileTree() {
  const tree = useWorkspace((s) => s.tree);
  const index = useWorkspace((s) => s.index);
  const gitStatus = useWorkspace((s) => s.gitStatus);
  const filter = useUI((s) => s.treeFilter);
  const filterKeys = useKeyLabel("tree.filter");
  const setFilter = useUI((s) => s.setTreeFilter);
  const active = useTabs((s) => s.active);
  const runs = useRuns((s) => s.runs);
  const [closed, setClosed] = useState<Set<string>>(() => new Set());
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const requests = useMemo(() => byFile(index), [index]);
  const matches = useFilterMatches(filter, index);
  const visible = useMemo(
    () => flatten(tree, requests, { closed, expanded: active ? new Set([...expanded, active]) : expanded, filter, matches }),
    [tree, requests, closed, expanded, active, filter, matches],
  );
  const scroller = useRef<HTMLDivElement>(null);
  const [cursorAt, setCursor] = useState(0);
  const cursor = Math.min(cursorAt, Math.max(0, visible.length - 1));
  // The virtualizer's functions are not memoizable; this component is not compiled.
  // eslint-disable-next-line react-hooks/incompatible-library
  const v = useVirtualizer({ count: visible.length, getScrollElement: () => scroller.current, estimateSize: (i) => (visible[i].kind === "req" ? 24 : 26), overscan: 12 });
  const matchCount = filter ? visible.filter((r) => r.kind === "req").length : 0;
  const fileCount = filter ? visible.filter((r) => r.kind === "file").length : 0;

  const toggle = (set: Set<string>, key: string, update: (s: Set<string>) => void) => {
    const next = new Set(set);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    update(next);
  };

  const click = (row: Row, index?: number) => {
    if (index !== undefined) setCursor(index);
    if (row.kind === "dir") toggle(closed, row.path, setClosed);
    else if (row.kind === "file") {
      if (row.fileKind === "secrets") openSecrets();
      else void useTabs.getState().open(row.path);
      if (active === row.path) toggle(expanded, row.path, setExpanded);
    } else void registry.getCommand("editor.revealLine")?.run({ path: row.path, line: row.req.line });
  };

  return (
    <>
      <div className="side-head">
        <h2>Files</h2>
        <div style={{ display: "flex", gap: 2 }}>
          <button className="icon-btn" title="New file" onClick={() => void newFile("")}>
            <PlusIcon />
          </button>
          <button className="icon-btn" title="Import" onClick={() => useUI.getState().openPalette(">import")}>
            <ImportIcon />
          </button>
        </div>
      </div>
      <label className="filter">
        <SearchIcon size={12} style={{ color: "var(--faint)", flex: "none" }} />
        <input
          id="tree-filter"
          aria-label="Filter requests in all files"
          placeholder="Filter requests in all files"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => e.key === "Escape" && setFilter("")}
          spellCheck={false}
        />
        {filter ? <kbd className="filter-esc">Esc</kbd> : filterKeys && <kbd style={{ background: "none", color: "var(--faint)" }}>{filterKeys}</kbd>}
      </label>
      {filter && (
        <div style={{ padding: "0 16px 8px", fontSize: 11.5, color: "var(--muted)" }}>
          {matchCount} request{matchCount === 1 ? "" : "s"} in {fileCount} file{fileCount === 1 ? "" : "s"} · matches method, path, headers and body
        </div>
      )}
      <div
        className="tree"
        ref={scroller}
        role="tree"
        aria-label="Project files"
        tabIndex={0}
        aria-activedescendant={visible.length ? `tree-row-${cursor}` : undefined}
        onKeyDown={(e) => {
          // Keys of the tree itself, not of its menu (a portal, whose
          // events bubble here too).
          if (e.target !== e.currentTarget) return;
          const row = visible[cursor];
          if (!row) return;
          const move = (i: number) => {
            const next = Math.max(0, Math.min(visible.length - 1, i));
            setCursor(next);
            v.scrollToIndex(next);
          };
          const openState = row.kind === "dir" ? row.open : row.kind === "file" ? row.open : false;
          // The menu's actions, by their keys.
          const pressed = pressedAction(e.nativeEvent);
          const run = pressed && keyAction(row, pressed);
          if (run) {
            // The tree's own keys: not also the app's (⌥⌘R runs a test).
            e.preventDefault();
            e.stopPropagation();
            void Promise.resolve(run()).catch(report);
            return;
          }
          switch (e.key) {
            case "ArrowDown":
              move(cursor + 1);
              break;
            case "ArrowUp":
              move(cursor - 1);
              break;
            case "Home":
              move(0);
              break;
            case "End":
              move(visible.length - 1);
              break;
            case "ArrowRight":
            case "ArrowLeft":
              if (row.kind === "req") return;
              if ((e.key === "ArrowRight") !== openState) toggle(row.kind === "dir" ? closed : expanded, row.path, row.kind === "dir" ? setClosed : setExpanded);
              break;
            case "Enter":
              click(row);
              break;
            case "F10":
            case "ContextMenu":
              if (e.key === "F10" && !e.shiftKey) return;
              if (row.kind === "req") return;
              {
                const el = document.getElementById(`tree-row-${cursor}`);
                const r = el?.getBoundingClientRect();
                el?.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, clientX: (r?.left ?? 0) + 24, clientY: r?.bottom ?? 0 }));
              }
              break;
            default:
              return;
          }
          e.preventDefault();
        }}
      >
        <div style={{ height: v.getTotalSize(), position: "relative" }}>
          {v.getVirtualItems().map((item) => {
            const row = visible[item.index];
            const style = { position: "absolute" as const, top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)`, paddingLeft: 8 + row.depth * 14 };
            if (row.kind === "req") {
              const e = runs[row.path]?.entries[row.req.entry];
              const code = e?.calls?.at(-1)?.response?.status;
              return (
                <div
                  key={item.key}
                  id={`tree-row-${item.index}`}
                  className="tree-row req"
                  style={{ ...style, paddingLeft: 30 + row.depth * 14 }}
                  role="treeitem"
                  aria-level={row.depth + 1}
                  data-cursor={item.index === cursor || undefined}
                  onClick={() => click(row, item.index)}
                  title={row.req.title || row.req.url}
                >
                  <span className={`mth m-${row.req.method}`}>{row.req.method === "DELETE" ? "DEL" : row.req.method}</span>
                  <span className="name">
                    <Highlight text={reqLabel(row.req)} hl={row.hl && [Math.max(0, row.hl[0] - row.req.method.length - 1), Math.max(0, row.hl[1] - row.req.method.length - 1)]} />
                  </span>
                  {code !== undefined && <span className="code" style={{ color: code >= 400 ? "var(--fail)" : "var(--muted)" }}>{code}</span>}
                  {e && <i className="state" style={{ background: e.success ? "var(--pass)" : "var(--fail)" }} />}
                </div>
              );
            }
            const run = row.kind === "file" ? runs[row.path] : undefined;
            const state = run?.summary ? (run.summary.outcome === "passed" ? "var(--pass)" : run.summary.outcome === "canceled" ? "var(--faint)" : "var(--fail)") : undefined;
            const git = gitStatus[row.path];
            return (
              <ContextMenu.Root key={item.key}>
                <ContextMenu.Trigger asChild>
                  <div
                    id={`tree-row-${item.index}`}
                    className="tree-row"
                    style={style}
                    role="treeitem"
                    aria-level={row.depth + 1}
                    aria-expanded={row.open}
                    aria-selected={row.kind === "file" && row.path === active}
                    data-cursor={item.index === cursor || undefined}
                    onClick={() => click(row, item.index)}
                    onContextMenu={() => setCursor(item.index)}
                  >
                    {/* Folders, and files that hold requests, open and close. */}
                    <span className="chev">{(row.kind === "dir" || !!requests.get(row.path)?.length) && <ChevronDown size={10} className={row.open ? undefined : "shut"} />}</span>
                    <i className="ic" style={{ background: row.kind === "dir" ? "var(--faint)" : kindColor[row.fileKind] ?? "var(--faint)" }} />
                    <span className="name">{row.kind === "file" ? <Highlight text={row.name} hl={row.hl} /> : row.name}</span>
                    {git && <span className="git" style={{ color: git === "A" ? "var(--pass)" : "var(--warn)" }}>{git}</span>}
                    {state && <i className="state" style={{ background: state }} />}
                  </div>
                </ContextMenu.Trigger>
                <TreeMenu row={row} />
              </ContextMenu.Root>
            );
          })}
        </div>
      </div>
      {registry.getSlots("files.bottom").map((s) => (
        <s.render key={s.id} />
      ))}
      <div className="side-note">
        <LockIcon style={{ flex: "none", marginTop: 2 }} />
        <span>Local folder. No account, no cloud sync. Share it the way you share code.</span>
      </div>
    </>
  );
}

/** The keys of the row actions (tinykeys syntax), on the focused row. */
const rowKeys = {
  newRequest: "$mod+KeyN",
  newFile: "$mod+Alt+KeyN",
  duplicate: "$mod+KeyD",
  rename: "F2",
  copyPath: "$mod+Alt+KeyC",
  reveal: "$mod+Alt+KeyR",
  trash: "$mod+Backspace",
} as const;
type RowAction = keyof typeof rowKeys;

/** What a row action does, or undefined when it does not apply to row. */
function rowAction(row: Row, a: RowAction): (() => unknown) | undefined {
  if (row.kind === "req") return undefined;
  const isFile = row.kind === "file";
  // Secrets files are listed, never opened, copied or renamed.
  const secret = isFile && row.fileKind === "secrets";
  const dir = row.kind === "dir" ? row.path : row.path.split("/").slice(0, -1).join("/");
  switch (a) {
    case "newRequest":
      return row.kind === "dir" ? () => newRequestIn(row.path) : isRequestFile(row.path, row.fileKind) ? () => newRequest(row.path) : undefined;
    case "newFile":
      return () => newFile(dir);
    case "duplicate":
      return secret ? undefined : () => Workspace.Duplicate(row.path).then(() => useWorkspace.getState().refresh());
    case "rename":
      return secret ? undefined : () => rename(row.path);
    case "copyPath":
      return () => copyPath(row.path);
    case "reveal":
      return windowLook ? () => WorkspaceDesktop.Reveal(row.path) : undefined;
    case "trash":
      return windowLook ? () => WorkspaceDesktop.Trash(row.path) : undefined;
  }
}

/** A row action from the keyboard: Move to Trash asks first (a key is
 * easy to press in the wrong place). */
function keyAction(row: Row, a: RowAction): (() => unknown) | undefined {
  const run = rowAction(row, a);
  if (!run || a !== "trash") return run;
  return async () => {
    if (await confirm({ title: `Move ${row.path} to the Trash?`, message: row.kind === "dir" ? "The folder and everything in it." : "You can put it back from the Trash.", submit: "Move to Trash" })) {
      await run();
    }
  };
}

/** The row action pressed in e, if any. */
function pressedAction(e: KeyboardEvent): RowAction | undefined {
  const keys = fromEvent(e);
  if (!keys) return undefined;
  const k = normalize(keys);
  return (Object.keys(rowKeys) as RowAction[]).find((a) => normalize(rowKeys[a]) === k);
}

function TreeMenu({ row }: { row: Row }) {
  const runKeys = useKeyLabel("file.run");
  const index = useWorkspace((s) => s.index);
  const item = (text: string, run: () => unknown, hint?: string, disabled = false) => (
    <ContextMenu.Item className="menu-item" disabled={disabled} onSelect={() => void Promise.resolve(run()).catch(report)}>
      {text}
      {hint && <span className="hint">{hint}</span>}
    </ContextMenu.Item>
  );
  const isFile = row.kind === "file";
  const runnable = isFile && isRequestFile(row.path, row.fileKind);
  // An action with its keys, when it applies to the row.
  const act = (a: RowAction, text: string) => {
    const run = rowAction(row, a);
    return run && item(text, run, label(rowKeys[a]));
  };
  // A folder's request files (what Run folder runs).
  const files = row.kind === "dir" ? new Set(index.filter((r) => r.file.startsWith(`${row.path}/`)).map((r) => r.file)).size : 0;
  return (
    <ContextMenu.Portal>
      <ContextMenu.Content className="menu">
        <ContextMenu.Label className="menu-label mono">{row.kind === "dir" ? `${row.path}/` : row.path}</ContextMenu.Label>
        {runnable && item("Run file", () => runPath(row.path), runKeys)}
        {row.kind === "dir" && item("Run folder", () => runFolder(row.path), `${files} file${files === 1 ? "" : "s"}`, files === 0)}
        {act("newRequest", row.kind === "dir" ? "New request here" : "New request")}
        {runnable && registry.getCommand("copyas.sonde.file") && item("Copy as sonde command", () => registry.getCommand("copyas.sonde.file")?.run(row.path))}
        {act("newFile", "New file…")}
        <ContextMenu.Separator className="menu-sep" />
        {act("duplicate", "Duplicate")}
        {act("rename", "Rename")}
        {act("copyPath", "Copy path")}
        {act("reveal", isMac ? "Reveal in Finder" : "Reveal in file manager")}
        {windowLook && (
          <>
            <ContextMenu.Separator className="menu-sep" />
            <ContextMenu.Item className="menu-item danger" onSelect={() => void Promise.resolve(rowAction(row, "trash")?.()).catch(report)}>
              Move to Trash
              <span className="hint">{label(rowKeys.trash)}</span>
            </ContextMenu.Item>
          </>
        )}
      </ContextMenu.Content>
    </ContextMenu.Portal>
  );
}

/** The Go filter's matches for q (headers, bodies…), debounced; they
 * follow the tree as the project changes (index). */
function useFilterMatches(q: string, index: unknown): ReadonlyMap<string, ReadonlySet<number>> | undefined {
  const [state, setState] = useState<{ q: string; m: Map<string, Set<number>> }>();
  const query = q.trim();
  useEffect(() => {
    if (!query) return;
    let live = true;
    const t = setTimeout(() => {
      Workspace.Filter(query)
        .then((list) => {
          if (!live) return;
          const m = new Map<string, Set<number>>();
          for (const x of list ?? []) {
            const set = m.get(x.file) ?? new Set<number>();
            set.add(x.entry);
            m.set(x.file, set);
          }
          setState({ q: query, m });
        })
        .catch(() => undefined);
    }, 120);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [query, index]);
  return state?.q === query ? state.m : undefined;
}

function report(err: unknown) {
  useUI.getState().toast({ kind: "error", text: appError(err).message });
}

/** Files that hold requests: .hurl and .sonde. */
export function isRequestFile(path: string, kind: string): boolean {
  return kind === "request" || path.endsWith(".sonde");
}

/** A secrets file is never opened (its values never reach the page): its
 * variables are set in Environments, write-only. */
function openSecrets() {
  useUI.getState().setPanel("env");
  useUI.getState().toast({ kind: "info", text: "Secrets files are not opened here: their values never reach the app's page. Set a secret in Environments." });
}

/** Copies file's absolute path, synchronously: WebKit drops the user
 * gesture across an await. */
function copyPath(file: string) {
  const dir = useWorkspace.getState().project?.dir ?? "";
  const sep = dir.includes("\\") ? "\\" : "/";
  const text = dir ? dir.replace(/[\\/]$/, "") + sep + file.split("/").join(sep) : file;
  return serverMode ? navigator.clipboard.writeText(text) : Clipboard.SetText(text);
}

async function runPath(path: string) {
  await useTabs.getState().open(path);
  await useRuns.getState().run(path);
}

function runFolder(path: string) {
  // The Test run panel registers the folder run.
  const cmd = registry.getCommand("testrun.runFolder");
  if (cmd) void cmd.run(path);
  else useUI.getState().toast({ kind: "info", text: "Test runs are not available yet" });
}

async function newFile(dir: string) {
  const name = await ask({ title: "New file", label: "File name", value: "untitled.hurl", submit: "Create" });
  if (!name) return;
  try {
    const path = await Workspace.NewFile(dir, name);
    // Listed now rather than when the folder watcher catches up.
    await Promise.all([useTabs.getState().open(path), useWorkspace.getState().refresh()]);
  } catch (err) {
    report(err);
  }
}

/** A new request file in dir, with a request to start from. */
async function newRequestIn(dir: string) {
  const name = await ask({ title: "New request", label: "File name", value: "untitled.hurl", submit: "Create" });
  if (!name) return;
  try {
    const path = await Workspace.NewFile(dir, name);
    await Promise.all([useWorkspace.getState().refresh(), newRequest(path)]);
  } catch (err) {
    report(err);
  }
}

async function newRequest(path: string) {
  await useTabs.getState().open(path);
  const tab = useTabs.getState().tabs.find((t) => t.path === path);
  if (!tab) return;
  const f = await Workspace.NewRequest(path, "GET", "{{base_url}}/", tab.hash);
  if (f) await useTabs.getState().reload(path);
}

async function rename(path: string) {
  const name = await ask({ title: "Rename", label: "New name", value: path.split("/").at(-1) ?? "", submit: "Rename" });
  if (!name) return;
  try {
    await Workspace.Rename(path, name);
  } catch (err) {
    report(err);
  }
}
