// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, type KeyboardEvent } from "react";
import { isMac } from "../../../lib/mode";

export interface ComposerProps {
  disabled: boolean;
  onSend(data: string, binary: boolean, write: boolean): Promise<boolean>;
}

/** The message box: text or binary (hex digits), sent with ⌘↵, optionally
 * also written to the file as steps. */
export function Composer({ disabled, onSend }: ComposerProps) {
  const [data, setData] = useState("");
  const [binary, setBinary] = useState(false);
  const [write, setWrite] = useState(false);
  const send = async () => {
    if (!data || disabled) return;
    if (await onSend(data, binary, write)) setData("");
  };
  // ⌘↵ anywhere in the composer sends (not the request at the cursor).
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      e.stopPropagation();
      void send();
    }
  };
  return (
    <div className="composer" onKeyDown={onKey}>
      <textarea
        className="mono"
        aria-label="Message"
        placeholder={binary ? "Hex digits, e.g. 01 ff" : "Message text or JSON"}
        value={data}
        disabled={disabled}
        onChange={(e) => setData(e.target.value)}
      />
      <div className="composer-bar">
        <div className="segmented" role="group" aria-label="Message type">
          <button aria-pressed={!binary} onClick={() => setBinary(false)}>
            Text
          </button>
          <button aria-pressed={binary} onClick={() => setBinary(true)}>
            Binary
          </button>
        </div>
        <label className="write-check">
          <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} /> Also write to file
        </label>
        <button className="btn" disabled={disabled || !data} onClick={() => void send()}>
          Send <kbd>{isMac ? "⌘↵" : "Ctrl+↵"}</kbd>
        </button>
      </div>
      <p className="composer-hint mono">Also write to file adds: send: … and receive: 1</p>
    </div>
  );
}
