// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Text view: shows the tab's editor (views.ts keeps one per tab).

import { useEffect, useRef } from "react";
import { openView } from "./views";

export function TextEditor({ file }: { file: string }) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const view = openView(file);
    host.current?.replaceChildren(view.dom);
    view.requestMeasure();
    view.focus();
  }, [file]);
  return <div className="text-editor" ref={host} />;
}
