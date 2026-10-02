// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The request line: method, URL (with {{variable}} completion) and the
// Send split button: Send this request (as ⌘↵ in Text: the earlier
// requests' captures are reused), Run requests 1 to N, Run the file.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { useRuns } from "../../state/run";
import { formEdit } from "./edit";
import { tokens, type Token } from "./field-tokens";
import type { EntryModel } from "./model";
import { SuggestInput, varSuggester } from "./suggest-input";

const methods = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

/** A gRPC URL's parts: the address as usual, the /service/method path
 * (set by the pickers) greyed. */
export function grpcTokens(url: string): Token[] {
  const m = /^(.*?)(\/[\w.]+\/\w+)$/.exec(url);
  return m ? [...tokens(m[1]), { kind: "muted", text: m[2] }] : tokens(url);
}

/** A method's color class (as the editor colors methods). */
export function methodClass(m: string): string {
  const k = m.toLowerCase();
  return ["get", "post", "put", "patch", "delete"].includes(k) ? `m-${k}` : "m-other";
}

/** A small chevron in the current color. */
export function Chevron() {
  return (
    <svg className="chev" width="9" height="9" viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
      <path d="M2.5 4l2.5 2.5L7.5 4" />
    </svg>
  );
}

export function UrlBar({ file, entry, grpcHint }: { file: string; entry: EntryModel; grpcHint?: boolean }) {
  const n = entry.Index;
  const running = useRuns((s) => !!s.runs[file]?.running);
  const sendKeys = useKeyLabel("request.send");
  const runToKeys = useKeyLabel("request.runTo");
  const runKeys = useKeyLabel("file.run");
  const custom = !methods.includes(entry.Method);
  return (
    <div className="url-bar">
      <span className={`method-pick ${methodClass(entry.Method)}${grpcHint ? " fixed" : ""}`}>
        <select
          className="method"
          aria-label="HTTP method"
          value={entry.Method}
          disabled={grpcHint}
          onChange={(e) => void formEdit(file, { kind: "setMethod", entry: n, value: e.target.value })}
          style={{ width: `calc(${entry.Method.length}ch + ${grpcHint ? 20 : 36}px)` }}
        >
          {custom && <option value={entry.Method}>{entry.Method}</option>}
          {methods.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
        {!grpcHint && <Chevron />}
      </span>
      <span className={`url-field${grpcHint ? " has-hint" : ""}`}>
        <SuggestInput
          label="URL"
          className="mono"
          value={entry.URL}
          suggest={varSuggester(file)}
          tokenize={grpcHint ? grpcTokens : undefined}
          onCommit={(v) => v.trim() && void formEdit(file, { kind: "setURL", entry: n, value: v.trim() })}
        />
        {grpcHint && <span className="url-hint">path set by service + method</span>}
      </span>
      <div className="send-split">
        <button className="send" disabled={running} onClick={() => void useRuns.getState().send(file, n)}>
          Send {sendKeys && <kbd>{sendKeys}</kbd>}
        </button>
        <Menu.Root>
          <Menu.Trigger asChild>
            <button className="split" aria-label="More ways to run" disabled={running}>
              <Chevron />
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
              <Menu.Separator className="menu-sep" />
              <Menu.Item className="menu-item tall" onSelect={() => void useRuns.getState().run(file)}>
                <b>Run whole file</b>
                <span className="hint">{runKeys}</span>
                <span className="sub">Every request, then every assert.</span>
              </Menu.Item>
              <div className="menu-note">In Text view {sendKeys || "⌘↵"} sends the request under the cursor.</div>
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
    </div>
  );
}
