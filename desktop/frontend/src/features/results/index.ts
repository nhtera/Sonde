// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Results panel: registered as the results host's view, with Save
// response (⌥⌘S) and, in the window, Open response in default app.

import { registry } from "../../app/registry";
import { on } from "../../lib/events";
import { resetMockSpec } from "./actions";
import { openExternally, saveBody } from "./body-actions";
import { windowLook } from "../../lib/mode";
import { ResultsPanel } from "./results-panel";
import { useTabs } from "../../state/tabs";
import { useResults } from "./state";
import "./results.css";

registry.results({ render: ResultsPanel });

registry.command({
  id: "response.save",
  title: "Save response to file",
  when: () => !!useResults.getState().activeBody,
  run: () => {
    const b = useResults.getState().activeBody;
    if (b) void saveBody(b);
  },
});

registry.command({
  id: "response.openExternally",
  title: "Open response in default app",
  when: () => windowLook && !!useResults.getState().activeBody,
  run: () => {
    const b = useResults.getState().activeBody;
    if (b?.id) void openExternally(b.id);
  },
});

// The session belongs to its file: another file's results close it.
useTabs.subscribe((s) => {
  const session = useResults.getState().session;
  if (session && s.active !== session.file) useResults.getState().closeSession();
});

// Another project: its spec, its requests.
on("ws:opened", () => {
  resetMockSpec();
  useResults.setState({ picked: {}, tab: null, session: null, activeBody: null });
});
