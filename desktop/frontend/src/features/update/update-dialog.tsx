// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The update dialog: an update on offer (Install, Skip, Later), its
// progress, Restart to update, and why an update cannot go on. Every
// error offers the release page. The notes are text, never markup.

import * as Dialog from "@radix-ui/react-dialog";
import { Update, UpdateState, appError, type UpdateStatus } from "../../lib/api";
import { useUpdate } from "../../state/update";
import { useUI } from "../../state/ui";
import { openReleasePage, restartToUpdate } from "./restart";
import { errorWords, shortNotes } from "./texts";

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

export function UpdateDialog() {
  const open = useUpdate((s) => s.open);
  const s = useUpdate((s) => s.status);
  const current = useUpdate((s) => s.info?.version);
  // Between answers (after Skip): nothing to say.
  if (!open || !s || s.state === UpdateState.StateIdle || s.state === UpdateState.$zero) return null;
  const hide = () => useUpdate.getState().hide();
  return (
    <Dialog.Root open onOpenChange={(o) => !o && hide()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog dialog-pad update-dialog" style={{ width: 460 }} aria-describedby={undefined}>
          <Body s={s} current={current} hide={hide} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Body({ s, current, hide }: { s: UpdateStatus; current?: string; hide(): void }) {
  const later = (
    <button className="btn-ghost" onClick={hide}>
      Later
    </button>
  );
  const download = (
    <button className="btn" onClick={openReleasePage}>
      Download
    </button>
  );
  switch (s.state) {
    case UpdateState.StateAvailable: {
      const notes = shortNotes(s.notes);
      return (
        <>
          <Dialog.Title className="dialog-title">
            Sonde Desktop {s.version}
            {s.skipped ? " (skipped)" : ""} is available
          </Dialog.Title>
          {current && <p className="dialog-note">You have {current}.</p>}
          {notes.text && <pre className="update-notes">{notes.text}</pre>}
          {notes.cut && (
            <button className="btn-ghost update-more" onClick={openReleasePage}>
              Full notes
            </button>
          )}
          {!s.canInstall && <p className="dialog-note">{s.reason}</p>}
          <div className="dialog-actions">
            {later}
            {!s.skipped && (
              <button className="btn" onClick={() => void Update.Skip(s.version).then(hide, fail)}>
                Skip this version
              </button>
            )}
            {s.canInstall ? (
              <button className="btn-primary" autoFocus onClick={() => void Update.Install().catch(fail)}>
                {s.skipped ? "Install anyway" : "Install"}
              </button>
            ) : (
              download
            )}
          </div>
        </>
      );
    }
    case UpdateState.StateDownloading:
    case UpdateState.StateVerifying: {
      const pct = s.total > 0 ? Math.floor((s.written * 100) / s.total) : 0;
      return (
        <>
          <Dialog.Title className="dialog-title">Updating to Sonde Desktop {s.version}</Dialog.Title>
          <p className="dialog-note">{s.state === UpdateState.StateVerifying ? "Verifying the download…" : `Downloading… ${pct}%`}</p>
          <progress className="update-progress" max={100} value={pct} aria-label="Download progress" />
          <div className="dialog-actions">{later}</div>
        </>
      );
    }
    case UpdateState.StateReady:
      return (
        <>
          <Dialog.Title className="dialog-title">Sonde Desktop {s.version} is ready to install</Dialog.Title>
          <p className="dialog-note">Sonde restarts to install it.</p>
          {s.otherInstances > 0 && (
            <p className="dialog-error">
              Quit the other Sonde windows first ({s.otherInstances} open).
            </p>
          )}
          <div className="dialog-actions">
            {later}
            {s.otherInstances > 0 && (
              <button className="btn" onClick={() => void Update.Check().catch(fail)}>
                Check again
              </button>
            )}
            <button className="btn-primary" autoFocus disabled={s.otherInstances > 0} onClick={() => void restartToUpdate()}>
              Restart to update
            </button>
          </div>
        </>
      );
    case UpdateState.StateError:
      return (
        <>
          <Dialog.Title className="dialog-title">The update did not go through</Dialog.Title>
          <p className="dialog-error">{errorWords(s)}</p>
          {s.error && s.error !== errorWords(s) && <p className="dialog-note update-detail">{s.error}</p>}
          <div className="dialog-actions">
            <button className="btn-ghost" onClick={hide}>
              Close
            </button>
            {download}
          </div>
        </>
      );
    case UpdateState.StateChecking:
      return (
        <>
          <Dialog.Title className="dialog-title">Checking for updates…</Dialog.Title>
          <div className="dialog-actions">{later}</div>
        </>
      );
    case UpdateState.StateDisabled:
      return (
        <>
          <Dialog.Title className="dialog-title">Updates are off</Dialog.Title>
          <p className="dialog-note">{s.reason}</p>
          <div className="dialog-actions">
            <button className="btn-ghost" onClick={hide}>
              Close
            </button>
          </div>
        </>
      );
  }
  return (
    <>
      <Dialog.Title className="dialog-title">Sonde Desktop is up to date</Dialog.Title>
      <div className="dialog-actions">
        <button className="btn-ghost" onClick={hide}>
          Close
        </button>
      </div>
    </>
  );
}
