// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Picking an input: in the window, a native dialog whose handle Go stages
// (the page never sees the path); in the browser (server mode), a file
// input whose bytes are uploaded.

import { Dialogs, Imports, type ImportInput } from "../../lib/api";
import { serverMode } from "../../lib/mode";

/** The file inputs the browser accepts, by kind. */
const accept: Record<string, string> = {
  curl: ".curl,.sh,.txt",
  postman: ".json",
  opencollection: ".yml,.yaml,.json",
  http: ".http,.rest,.json,.env",
  openapi: ".yaml,.yml,.json",
};

/** Base64 of a file's bytes. */
export function base64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

/** Picks a file (or a folder, window only) and stages it; null when
 * canceled. */
export async function pickInput(kind: string, title: string, dir = false): Promise<ImportInput | null> {
  if (!serverMode) {
    const handle = dir ? await Dialogs.OpenFolder(title) : await Dialogs.OpenFile(title, "", "");
    return handle ? Imports.Stage(handle, dir ? "dir" : "file") : null;
  }
  const file = await chooseFile(accept[kind] ?? "");
  if (!file) return null;
  return Imports.Upload(file.name, base64(new Uint8Array(await file.arrayBuffer())));
}

function chooseFile(accept: string): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = accept;
    input.onchange = () => resolve(input.files?.[0] ?? null);
    input.oncancel = () => resolve(null);
    input.click();
  });
}
