// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { KvGrid } from "../kv-grid";
import type { EntryModel } from "../model";

/** Query parameters, written to [Query] (the URL stays readable). */
export function ParamsTab({ file, entry }: { file: string; entry: EntryModel }) {
  return (
    <div className="form-tab">
      <KvGrid file={file} entry={entry} sec="query" addLabel="+ Add parameter" />
      <p className="form-note">Parameters are written to [Query]; the URL keeps its own query string.</p>
    </div>
  );
}
