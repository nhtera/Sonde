// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The column names of a data file, as a data-driven run names its
// variables: a CSV file's header, or the keys of a JSON array's first
// object.

/** The variables a data file defines (none when it does not read). */
export function dataColumns(path: string, text: string): string[] {
  if (/\.json$/i.test(path)) {
    try {
      const rows = JSON.parse(text) as unknown;
      const first = Array.isArray(rows) ? rows[0] : undefined;
      return first && typeof first === "object" ? Object.keys(first as object).sort() : [];
    } catch {
      return [];
    }
  }
  return csvHeader(text);
}

/** The first record of a CSV text, its fields trimmed. */
function csvHeader(text: string): string[] {
  const out: string[] = [];
  let field = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        field += '"';
        i++;
      } else if (c === '"') quoted = false;
      else field += c;
    } else if (c === '"') quoted = true;
    else if (c === ",") {
      out.push(field.trim());
      field = "";
    } else if (c === "\n" || c === "\r") break;
    else field += c;
  }
  out.push(field.trim());
  return out.filter(Boolean);
}
