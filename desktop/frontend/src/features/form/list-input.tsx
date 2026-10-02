// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Chevron } from "./url-bar";

/** A field that picks from a list (a datalist) and takes any other text:
 * a chevron in the field's color says so. It commits on blur and Enter. */
export function ListInput({ label, list, value, className, placeholder, onCommit }: { label: string; list: string; value: string; className: string; placeholder?: string; onCommit(v: string): void }) {
  return (
    <span className={`list-pick ${className}`}>
      <input
        className="mono"
        aria-label={label}
        list={list}
        defaultValue={value}
        key={value}
        placeholder={placeholder}
        spellCheck={false}
        onBlur={(e) => e.target.value.trim() && e.target.value.trim() !== value && onCommit(e.target.value.trim())}
        onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
      />
      <Chevron />
    </span>
  );
}
