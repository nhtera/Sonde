// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The request at a cursor, and each request's shape, from Go (editsvc):
// the entry index never comes from the highlighting tree.

import { EditSvc } from "../../lib/api";
import type { EntryShape } from "./results/marks";

export interface Buffer {
  file: string;
  text: string;
  version: number;
}

const models = new Map<string, { version: number; model: Promise<EntryShape[] | null> }>();
/** Each file's last model that parsed, with its text. */
const parsed = new Map<string, { model: EntryShape[]; text: string }>();

/** The entries of a buffer (cached per file and version), or null while
 * the text does not parse (the marks set earlier then stay). */
export function modelOf(b: Buffer): Promise<EntryShape[] | null> {
  const had = models.get(b.file);
  if (had && had.version === b.version) return had.model;
  const model = EditSvc.Model(b).then(
    (m) => {
      const shape = (m ?? []) as unknown as EntryShape[];
      parsed.set(b.file, { model: shape, text: b.text });
      return shape;
    },
    () => null,
  );
  models.set(b.file, { version: b.version, model });
  return model;
}

/** The file's last model that parsed and the text it is of: while a
 * request is being typed the file may not parse, and what comes before
 * the edit is still there. */
export function lastParsed(file: string): { model: EntryShape[]; text: string } | undefined {
  return parsed.get(file);
}

/** Forgets a closed file's model. */
export function dropModel(file: string) {
  models.delete(file);
  parsed.delete(file);
}

/** The 1-based entry holding the UTF-16 offset, or 0 when none does. */
export async function entryAt(b: Buffer, offset: number): Promise<number> {
  // The model answers most cursors without another call.
  const model = await modelOf(b);
  const hit = model?.find((e) => offset >= e.Range.Start && offset <= e.Range.End);
  if (hit) return hit.Index;
  return EditSvc.EntryAt(b, offset).catch(() => 0);
}
