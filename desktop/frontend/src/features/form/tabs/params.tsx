// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { KvGrid } from "../kv-grid";
import type { EntryModel } from "../model";

/** Query parameters, written to [Query] (the URL stays readable: the
 * write-back preview says so). */
export function ParamsTab({ file, entry }: { file: string; entry: EntryModel }) {
  return (
    <div className="form-tab">
      <KvGrid file={file} entry={entry} sec="query" />
    </div>
  );
}
