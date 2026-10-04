// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Clipboard } from "@wailsio/runtime";
import { serverMode } from "./mode";

/** Puts text on the clipboard. The window app goes through Go: WebKit
 * refuses navigator.clipboard once a click's gesture is spent, as it is
 * across an await (a copy that first asks Go for its text). */
export function copyText(text: string): Promise<unknown> {
  return serverMode ? navigator.clipboard.writeText(text) : Clipboard.SetText(text);
}
