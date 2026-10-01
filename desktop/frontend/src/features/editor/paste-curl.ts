// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry } from "../../app/registry";

/** Opens the curl import with text (a curl command) in it; text that is
 * not one opens the form empty. */
export async function pasteAsCurl(text: string) {
  await registry.getCommand("import.curl")?.run();
  if (text.trim().startsWith("curl")) await registry.getCommand("import.paste")?.run(text);
}
