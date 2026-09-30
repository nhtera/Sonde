// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect } from "react";
import { AskHost } from "../components/ask";
import { Palette } from "../features/palette/palette";
import { Toasts } from "../features/shell/toasts";
import { appError } from "../lib/api";
import { startLspSession } from "../lib/lsp-session";
import { useEnv } from "../state/env";
import { useSettings } from "../state/settings";
import { isDirty, useTabs } from "../state/tabs";
import { useUI } from "../state/ui";
import { useWorkspace } from "../state/workspace";
import { bindKeys } from "./keymap/keymap-manager";
import { ShortcutsSheet } from "./keymap/shortcuts-sheet";
import { useKeys } from "./keymap/use-keys";
import { Layout } from "./layout";

export function App() {
  const keys = useKeys();

  useEffect(() => {
    const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });
    useSettings.getState().load().catch(fail);
    useWorkspace.getState().load().catch(fail);
    useEnv.getState().load().catch(fail);
    return startLspSession();
  }, []);

  useEffect(() => bindKeys(window, keys), [keys]);

  // Unsaved edits: the page asks before it goes away.
  useEffect(() => {
    const guard = (e: BeforeUnloadEvent) => {
      if (useTabs.getState().tabs.some(isDirty)) e.preventDefault();
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, []);

  return (
    <>
      <Layout />
      <Palette />
      <ShortcutsSheet />
      <AskHost />
      <Toasts />
    </>
  );
}
