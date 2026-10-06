// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from "zustand";
import { appError, Envs, type EnvProject, type Overrides } from "../lib/api";
import { on } from "../lib/events";
import { useUI } from "./ui";

interface EnvState {
  project: EnvProject | null;
  /** The environment runs use ("" when the project has none). */
  current: string;
  overrides: Overrides;
  load(): Promise<void>;
  select(env: string): void;
}

/** Counts the projects opened: a load started before another project
 * opened is of the previous one. */
let opened = 0;

export const useEnv = create<EnvState>((set, get) => ({
  project: null,
  current: "",
  overrides: { count: 0, items: [] },
  load: async () => {
    const at = opened;
    // No folder open (the welcome screen): no environments, not an error.
    const list = Envs.List().catch((err: unknown) => {
      if (appError(err).code === "not-found") return null;
      throw err;
    });
    const [project, overrides] = await Promise.all([list, Envs.Overrides()]);
    // Another project opened meanwhile: these are the previous one's.
    if (at !== opened) return;
    let current = get().current;
    const names = (project?.envs ?? []).map((e) => e.name);
    if (!names.includes(current)) current = project?.envs?.find((e) => e.default)?.name ?? names[0] ?? "";
    set({ project, overrides, current });
  },
  select: (current) => set({ current }),
}));

/** Whether a change to paths (project paths) changes the environments:
 * sonde.yaml, or a file its environments read. */
export function touchesEnvs(project: EnvProject | null, paths: string[]): boolean {
  const files = new Set(["sonde.yaml", "sonde.yml"]);
  for (const e of project?.envs ?? []) {
    if (e.secretsFile) files.add(e.secretsFile);
    for (const v of e.variables ?? []) files.add(v.source);
  }
  return paths.some((p) => files.has(p));
}

on("env:changed", () => void useEnv.getState().load());
// A secret written in a git repository: its file is kept out of git.
on("env:gitignored", (data) => {
  const { line, file } = data as { line: string; file: string };
  useUI.getState().toast({ kind: "success", text: `Added ${line} to .gitignore: ${file} stays out of git` });
});
// Written outside the app, or by an import.
on("ws:changed", (data) => {
  if (touchesEnvs(useEnv.getState().project, (data as { paths: string[] }).paths)) void useEnv.getState().load();
});
// Settings that change a run count as overrides.
on("settings:changed", () => void useEnv.getState().load());
on("ws:opened", () => {
  opened++;
  // The previous project's environments are not shown meanwhile.
  useEnv.setState({ project: null, current: "" });
  void useEnv.getState().load();
});
