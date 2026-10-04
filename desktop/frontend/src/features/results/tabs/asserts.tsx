// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The checks of a request: the first failure explained, then each assert
// line with its result, and the contract's violations (rows, not errors).

import { useKeyLabel } from "../../../app/keymap/use-keys";
import { FailureBox } from "../../../components/run/failure-box";
import { failureOf } from "../../../components/run/model";
import { defineVariable } from "../../panels/env/define-variable";
import type { Entry } from "../../../lib/view";
import { runCommand } from "../actions";
import { assertRows } from "../model";

export function AssertsTab({ entry, lines, onShowResponse }: { entry: Entry; lines: string[]; onShowResponse?(): void }) {
  const gotoKeys = useKeyLabel("editor.gotoLine");
  const failure = failureOf(entry, lines);
  const asserts = assertRows(entry);
  const violations = entry.sonde?.contract?.violations ?? [];
  if (!failure && asserts.length === 0 && violations.length === 0) return <p className="body-note tab-pad">This request has no asserts.</p>;
  return (
    <div className="tab-pad">
      {failure && (
        <FailureBox failure={failure} gotoKeys={gotoKeys} onGoto={(line) => runCommand("editor.gotoLine", line)} onShowResponse={onShowResponse} onDefine={defineVariable} />
      )}
      <ul className="assert-rows">
        {asserts
          .filter((a) => !(failure && !a.success && a.line === failure.line))
          .map((a, i) => (
            <li key={i} className={a.success ? "pass" : "fail"}>
              <span className="mark">{a.success ? "✓" : "✕"}</span>
              <span className="line mono">L{a.line}</span>
              <span className="code mono">{lines[a.line - 1]?.trim() ?? ""}</span>
              {a.message && <span className="msg mono">{a.message}</span>}
            </li>
          ))}
        {violations.map((v, i) => (
          <li key={`v${i}`} className={`spec ${v.warning ? "warn" : "fail"}`}>
            <div>
              <span className="spec-chip">spec</span> <b>Contract {v.warning ? "warning" : "violation"}</b>{" "}
              <span className="muted">· an Asserts row, not an error</span>
            </div>
            <span className="msg mono">
              {v.message}
              {v.instance_path ? ` at ${v.instance_path}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
