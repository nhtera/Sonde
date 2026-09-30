// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Saving a response and opening it in another app. The window saves the
// raw bytes received (Go writes them to the file picked); server mode has
// no dialogs, so the browser downloads the redacted body.

import { appError, BodiesDesktop } from "../../lib/api";
import { serverMode } from "../../lib/mode";
import { useUI } from "../../state/ui";
import { bodyURL } from "./tabs/body/body-fetch";

const toast = (kind: "success" | "error", text: string) => useUI.getState().toast({ kind, text });

/** Saves body id to a file. */
export async function saveBody(body: { id: string; contentType: string }) {
  if (!body.id) return;
  if (serverMode) {
    const a = document.createElement("a");
    a.href = bodyURL(body.id);
    a.download = "response";
    a.click();
    return;
  }
  try {
    const name = await BodiesDesktop.SaveResponse(body.id);
    if (name) toast("success", `Saved ${name}`);
  } catch (err) {
    toast("error", appError(err).message);
  }
}

/** Opens body id (redacted) in the system's app for its type. */
export async function openExternally(id: string) {
  try {
    await BodiesDesktop.OpenExternally(id);
  } catch (err) {
    toast("error", appError(err).message);
  }
}
