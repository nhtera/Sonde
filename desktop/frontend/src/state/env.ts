// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from "zustand";
import { Envs, type EnvProject, type Overrides } from "../lib/api";
import { on } from "../lib/events";

interface EnvState {
  project: EnvProject | null;
  /** The environment runs use ("" when the project has none). */
  current: string;
  overrides: Overrides;
  load(): Promise<void>;
  select(env: string): void;
}

export const useEnv = create<EnvState>((set, get) => ({
  project: null,
  current: "",
  overrides: { count: 0, items: [] },
  load: async () => {
    const [project, overrides] = await Promise.all([Envs.List(), Envs.Overrides()]);
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
// Written outside the app, or by an import.
on("ws:changed", (data) => {
  if (touchesEnvs(useEnv.getState().project, (data as { paths: string[] }).paths)) void useEnv.getState().load();
});
// Settings that change a run count as overrides.
on("settings:changed", () => void useEnv.getState().load());
on("ws:opened", () => {
  useEnv.setState({ current: "" });
  void useEnv.getState().load();
});
