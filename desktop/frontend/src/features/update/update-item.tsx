// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The status bar's update item: an update on offer, its progress, Restart
// to update, an update that did not verify, the test release server.

import { UpdateState } from "../../lib/api";
import { unverified, useUpdate } from "../../state/update";
import { restartToUpdate } from "./restart";

export function UpdateItem() {
  const s = useUpdate((st) => st.status);
  const test = useUpdate((st) => st.info?.testServer);
  const show = () => useUpdate.getState().show();
  let item = null;
  if (s?.state === UpdateState.StateAvailable) {
    item = (
      <button className="update-item" onClick={show}>
        Update {s.version}
      </button>
    );
  } else if (s?.state === UpdateState.StateDownloading || s?.state === UpdateState.StateVerifying) {
    const pct = s.total > 0 ? Math.floor((s.written * 100) / s.total) : 0;
    item = (
      <button className="update-item" onClick={show}>
        Updating… {pct}%
      </button>
    );
  } else if (s?.state === UpdateState.StateReady) {
    item = (
      <button className="update-item" onClick={() => (s.otherInstances > 0 ? show() : void restartToUpdate())}>
        Restart to update
      </button>
    );
  } else if (unverified(s)) {
    item = (
      <button className="update-item" data-warn onClick={show}>
        Update not verified
      </button>
    );
  }
  return (
    <>
      {test && <span className="update-tag">Test update server</span>}
      {item}
    </>
  );
}
