// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The update texts the page shows. The privacy ones are the docs' words
// (docs/desktop.md › Updates); a test compares them.

import { UpdateErrorKind, UpdateState, type UpdateStatus } from "../../lib/api";

/** What the update check sends, under its switch. */
export const updateCheckNote =
  "Once a day Sonde asks GitHub which Sonde Desktop versions exist (api.github.com). When one is newer, it downloads that version's signed description from github.com. These requests send nothing about you, your projects or your computer; GitHub sees your IP address, as with any download.";

/** The privacy line: the window app's, which has the update check, and
 * server mode's. */
export const privacyLine = (updates: boolean) =>
  updates
    ? "No account and no telemetry. Sonde sends the requests you write, plus the update check when it is on."
    : "No account and no telemetry. Sonde sends the requests you write.";

/** The answer of the last check, for Settings. */
export function lastResult(s: UpdateStatus | null): string {
  switch (s?.state) {
    case UpdateState.StateUpToDate:
      return "up to date";
    case UpdateState.StateAvailable:
      return `${s.version} available`;
    case UpdateState.StateReady:
      return `${s.version} ready to install`;
    case UpdateState.StateError:
      return errorWords(s);
  }
  return "";
}

/** An update error in plain words, by kind. */
export function errorWords(s: UpdateStatus): string {
  switch (s.errorKind) {
    case UpdateErrorKind.KindNetwork:
      return "Could not reach GitHub.";
    case UpdateErrorKind.KindVerification:
      return "The update could not be verified and was not installed.";
    case UpdateErrorKind.KindRelease:
      return "This release is not complete yet. Try again later.";
  }
  return s.error || "The update failed.";
}

/** The notes as shown: text, at most about 4 KB. */
export function shortNotes(notes: string): { text: string; cut: boolean } {
  const max = 4096;
  return notes.length > max ? { text: notes.slice(0, max).trimEnd() + "…", cut: true } : { text: notes, cut: false };
}
