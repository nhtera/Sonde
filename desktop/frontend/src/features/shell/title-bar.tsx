// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry } from "../../app/registry";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { BranchIcon, ChevronDown, PlayIcon, SearchIcon, SondeMark } from "../../components/icons";
import { isMac, serverMode, windowLook } from "../../lib/mode";
import { useEnv } from "../../state/env";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";
import { envColor } from "../../lib/env-color";
import { useKeyLabel } from "../../app/keymap/use-keys";

export function TitleBar() {
  const project = useWorkspace((s) => s.project);
  const git = useWorkspace((s) => s.git);
  const active = useTabs((s) => s.active);
  const running = useRuns((s) => (active ? s.runs[active]?.running : false));
  const paletteKeys = useKeyLabel("palette.open");
  const runKeys = useKeyLabel("file.run");
  return (
    <header className="titlebar">
      {isMac && windowLook && (
        <div className="traffic">
          {/* The window draws its own; the harness draws them for the design screens. */}
          {serverMode && (
            <>
              <i />
              <i />
              <i />
            </>
          )}
        </div>
      )}
      <div className="project">
        <SondeMark />
        <ProjectSwitcher name={project?.name ?? "Sonde"} />
        {git?.repo && git.branch && (
          <span className="branch" title="git branch">
            <BranchIcon />
            {git.branch}
          </span>
        )}
      </div>
      <div style={{ flex: 1, display: "flex", justifyContent: "center", minWidth: 0 }}>
        <button className="search-field" onClick={() => useUI.getState().openPalette()} aria-label="Search files and commands">
          <SearchIcon />
          <span>Search files and commands</span>
          {paletteKeys && <kbd>{paletteKeys}</kbd>}
        </button>
      </div>
      {project && (
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <OverridesChip />
          <EnvPicker />
          <button
            className="btn-primary"
            disabled={!active || running}
            onClick={() => active && void useRuns.getState().run(active)}
            title="Run every request of the file"
          >
            <PlayIcon />
            Run file{runKeys && <kbd>{runKeys}</kbd>}
          </button>
        </div>
      )}
    </header>
  );
}

/** When a folder was last opened: "2 h ago", "yesterday", "last week". */
export function openedWhen(iso: string, now = new Date()): string {
  const h = Math.floor((now.getTime() - new Date(iso).getTime()) / 3_600_000);
  if (Number.isNaN(h)) return "";
  if (h < 1) return "just now";
  if (h < 24) return `${h} h ago`;
  const d = Math.floor(h / 24);
  if (d === 1) return "yesterday";
  if (d < 7) return `${d} days ago`;
  if (d < 14) return "last week";
  return new Date(iso).toLocaleDateString();
}

function ProjectSwitcher({ name }: { name: string }) {
  const recent = useWorkspace((s) => s.recent);
  const here = useWorkspace((s) => s.project?.dir);
  const openKeys = useKeyLabel("folder.open");
  const importKeys = useKeyLabel("import.open");
  if (!windowLook) return <span style={{ fontWeight: 600 }}>{name}</span>;
  // The folder open first, then the others, latest first.
  const folders = [...recent].sort((a, b) => Number(b.dir === here) - Number(a.dir === here) || Date.parse(b.openedAt) - Date.parse(a.openedAt));
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="chip-btn">
          {name}
          <ChevronDown style={{ color: "var(--muted)" }} />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu project-menu" align="start" sideOffset={6}>
          {folders.length > 0 && <Menu.Label className="menu-label">Recent folders</Menu.Label>}
          {folders.map((r) => (
            <Menu.Item key={r.id} className={`menu-item folder${r.dir === here ? " current" : ""}`} onSelect={() => r.dir !== here && void useWorkspace.getState().openRecent(r.id)} title={r.dir}>
              <span className="folder-name">{r.name}</span>
              <span className="folder-when" data-volatile={r.dir === here ? undefined : true}>
                {r.dir === here ? "current" : openedWhen(r.openedAt)}
              </span>
              <span className="folder-dir mono">{r.dir}</span>
            </Menu.Item>
          ))}
          {folders.length > 0 && <Menu.Separator className="menu-sep" />}
          <Menu.Item className="menu-item" onSelect={() => void useWorkspace.getState().openFolder()}>
            Open folder…<span className="hint">{openKeys}</span>
          </Menu.Item>
          {registry.getCommand("import.open") && (
            <Menu.Item className="menu-item" onSelect={() => void registry.getCommand("import.open")?.run()}>
              Import collection…<span className="hint">{importKeys}</span>
            </Menu.Item>
          )}
          <div className="menu-note">Each folder has its own sonde.yaml, history and cookie jar.</div>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

const noEnvs: never[] = [];

function EnvPicker() {
  const envs = useEnv((s) => s.project?.envs) ?? noEnvs;
  const current = useEnv((s) => s.current);
  if (envs.length === 0) return null;
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="env-picker" aria-label="Environment">
          <i className="dot" style={{ background: envColor(current) }} />
          {current}
          <ChevronDown style={{ color: "var(--muted)" }} />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu" align="end" sideOffset={6}>
          <Menu.Label className="menu-label">Environment</Menu.Label>
          {envs.map((e) => (
            <Menu.Item key={e.name} className="menu-item" onSelect={() => useEnv.getState().select(e.name)}>
              <i className="dot" style={{ background: envColor(e.name) }} />
              {e.name}
              {e.name === current && <span className="hint">✓</span>}
            </Menu.Item>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}


export function OverridesChip() {
  const o = useEnv((s) => s.overrides);
  if (o.count === 0) return null;
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="overrides-chip" title="Values that change runs beyond the project files">
          {o.count} override{o.count === 1 ? "" : "s"}
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu" align="end" sideOffset={6} style={{ minWidth: 300 }}>
          <Menu.Label className="menu-label">Overrides of every run</Menu.Label>
          {(o.items ?? []).map((it, i) => (
            <div key={i} className="menu-item" style={{ height: "auto", padding: "5px 8px", flexDirection: "column", alignItems: "flex-start", gap: 2 }}>
              <span>
                {it.name} <span style={{ color: "var(--faint)" }}>· {it.source}</span>
              </span>
              <code style={{ fontSize: 11, color: "var(--muted)" }}>{it.flag}</code>
            </div>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
