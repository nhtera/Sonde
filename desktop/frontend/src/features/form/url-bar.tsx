// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The request line: method, URL (with {{variable}} completion) and the
// Send split button: Send this request (as ⌘↵ in Text: the earlier
// requests' captures are reused), Run requests 1 to N, Run the file.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { useRuns } from "../../state/run";
import { formEdit } from "./edit";
import type { EntryModel } from "./model";
import { SuggestInput, varSuggester } from "./suggest-input";

const methods = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

export function UrlBar({ file, entry, grpcHint }: { file: string; entry: EntryModel; grpcHint?: boolean }) {
  const n = entry.Index;
  const running = useRuns((s) => !!s.runs[file]?.running);
  const sendKeys = useKeyLabel("request.send");
  const runToKeys = useKeyLabel("request.runTo");
  const runKeys = useKeyLabel("file.run");
  const custom = !methods.includes(entry.Method);
  return (
    <div className="url-bar">
      <select
        className="method mono"
        aria-label="HTTP method"
        value={entry.Method}
        disabled={grpcHint}
        onChange={(e) => void formEdit(file, { kind: "setMethod", entry: n, value: e.target.value })}
      >
        {custom && <option value={entry.Method}>{entry.Method}</option>}
        {methods.map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
      </select>
      <span className="url-field">
        <SuggestInput
          label="URL"
          className="mono"
          value={entry.URL}
          suggest={varSuggester(file)}
          onCommit={(v) => v.trim() && void formEdit(file, { kind: "setURL", entry: n, value: v.trim() })}
        />
      </span>
      <div className="send-split">
        <button className="btn-primary" disabled={running} onClick={() => void useRuns.getState().send(file, n)}>
          Send {sendKeys && <kbd>{sendKeys}</kbd>}
        </button>
        <Menu.Root>
          <Menu.Trigger asChild>
            <button className="btn-primary split" aria-label="More ways to run" disabled={running}>
              ▾
            </button>
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="menu send-menu" align="end" sideOffset={4}>
              <Menu.Item className="menu-item tall" onSelect={() => void useRuns.getState().send(file, n)}>
                <b>Send this request</b>
                <span className="hint">{sendKeys}</span>
                <span className="sub">Request {n} only. Reuses the captures of the file's last run.</span>
              </Menu.Item>
              <Menu.Item className="menu-item tall" onSelect={() => void useRuns.getState().run(file, n)}>
                <b>Run requests 1 to {n}</b>
                <span className="hint">{runToKeys}</span>
                <span className="sub">Fresh captures from the top. Same as sonde --to-entry {n}.</span>
              </Menu.Item>
              <Menu.Item className="menu-item tall" onSelect={() => void useRuns.getState().run(file)}>
                <b>Run whole file</b>
                <span className="hint">{runKeys}</span>
                <span className="sub">Every request, then every assert.</span>
              </Menu.Item>
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
    </div>
  );
}
