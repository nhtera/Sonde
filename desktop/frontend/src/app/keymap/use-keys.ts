// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useMemo } from "react";
import { useSettings } from "../../state/settings";
import { useRegistry } from "../registry";
import { effectiveKeys, label } from "./keymap-manager";

/** Every command's keys in effect, updated on rebinding. */
export function useKeys(): Record<string, string> {
  const version = useRegistry();
  const user = useSettings((s) => s.value?.shortcuts);
  // version: commands registered later get their keys too.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => effectiveKeys(user ?? undefined), [user, version]);
}

/** The label of a command's keys ("⌘K"), or "" when it has none. */
export function useKeyLabel(command: string): string {
  const k = useKeys()[command];
  return k ? label(k) : "";
}
