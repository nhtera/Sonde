// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The update service's state (window app), from Update.State() and the
// "update:state" events. Every status carries a rising seq: one that is
// not newer than the last is dropped, so a late event never undoes a
// later state.

import { create } from "zustand";
import { Update, UpdateState, UpdateErrorKind, appError, type AppInfo, type UpdateStatus } from "../lib/api";
import { on } from "../lib/events";
import { useUI } from "./ui";

interface UpdateStore {
  status: UpdateStatus | null;
  info: AppInfo | null;
  /** The update dialog is open. */
  open: boolean;
  /** A background find opened the dialog this session: the next ones toast. */
  shownFind: boolean;
  /** The last version a background find showed: the same one again (the
   * next day, after being offline) is not news. */
  lastOffered: string;
  apply(s: UpdateStatus): void;
  show(): void;
  hide(): void;
}

export const useUpdate = create<UpdateStore>((set, get) => ({
  status: null,
  info: null,
  open: false,
  shownFind: false,
  lastOffered: "",
  apply: (s) => {
    const cur = get().status;
    if (cur && s.seq <= cur.seq) return;
    set({ status: s });
    react(s);
  },
  show: () => set({ open: true }),
  hide: () => set({ open: false }),
}));

/** What a new status shows: the dialog for a manual check's answer and
 * the first background find of the session; a toast otherwise. */
function react(s: UpdateStatus) {
  const st = useUpdate.getState();
  if (s.manual) {
    if (s.state === UpdateState.StateUpToDate) {
      useUI.getState().toast({ kind: "info", text: `Sonde Desktop is up to date (${st.info?.version ?? "this version"}).` });
    } else if ([UpdateState.StateAvailable, UpdateState.StateError, UpdateState.StateDisabled, UpdateState.StateReady].includes(s.state)) {
      st.show();
    }
    return;
  }
  if (s.state === UpdateState.StateAvailable && s.version !== st.lastOffered) {
    useUpdate.setState({ lastOffered: s.version });
    if (!st.shownFind) {
      useUpdate.setState({ shownFind: true, open: true });
    } else {
      useUI.getState().toast({ kind: "info", text: `Sonde Desktop ${s.version} is available`, action: { label: "View", run: st.show } });
    }
  }
}

/** Whether the status is an update that did not verify (shown even from a
 * background check). */
export const unverified = (s: UpdateStatus | null) => s?.state === UpdateState.StateError && s.errorKind === UpdateErrorKind.KindVerification;

/** Loads the state and follows its events; returns the cleanup. */
export function startUpdates(): () => void {
  const off = on("update:state", (data) => useUpdate.getState().apply(data as UpdateStatus));
  const fail = (err: unknown) => console.warn("update:", appError(err).message);
  Update.Info()
    .then((info) => useUpdate.setState({ info }))
    .catch(fail);
  // The state so far, shown as is: only new events open or toast.
  Update.State()
    .then((s) => {
      const cur = useUpdate.getState().status;
      if (!cur || s.seq > cur.seq) useUpdate.setState({ status: s });
    })
    .catch(fail);
  return off;
}
