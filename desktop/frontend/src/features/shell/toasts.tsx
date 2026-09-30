// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { CloseIcon } from "../../components/icons";
import { useUI } from "../../state/ui";

export function Toasts() {
  const toasts = useUI((s) => s.toasts);
  return (
    <div className="toasts">
      {toasts.map((t) => (
        // Errors are announced at once; the rest politely.
        <div key={t.id} className="toast" data-kind={t.kind} role={t.kind === "error" ? "alert" : "status"}>
          <span>{t.text}</span>
          {t.action && (
            <button
              onClick={() => {
                t.action!.run();
                useUI.getState().dismiss(t.id);
              }}
            >
              {t.action.label}
            </button>
          )}
          <button aria-label="Dismiss" style={{ color: "var(--faint)" }} onClick={() => useUI.getState().dismiss(t.id)}>
            <CloseIcon />
          </button>
        </div>
      ))}
    </div>
  );
}
