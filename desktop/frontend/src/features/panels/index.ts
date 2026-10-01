// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The rail's panels: History, Test run, Environments, Contract & mock, AI
// agents and Settings; the Changes card under the file tree; the cookie
// jar; and the commands other features call (open the environments, the
// settings at a section, the cookie jar).

import { registry } from "../../app/registry";
import { AgentsIcon, ContractIcon, EnvIcon, HistoryIcon, SettingsIcon, TestRunIcon } from "../../components/icons";
import { useUI } from "../../state/ui";
import { AgentsPanel } from "./agents/agents-panel";
import { ContractPanel } from "./contract/contract-panel";
import { CookieJarDialog, useCookieJar } from "./cookies/cookie-jar";
import { EnvMain, EnvSide } from "./env/env-panel";
import { ChangesCard } from "./git/changes-card";
import { HistoryPanel } from "./history/history-panel";
import { goToSection, SettingsMain, SettingsSide } from "./settings/settings-panel";
import { TestRunMain } from "./testrun/test-run-main";
import { TestRunSide } from "./testrun/test-run-side";
import "./panels.css";

registry.panel({ id: "history", title: "History", icon: HistoryIcon, order: 1, render: HistoryPanel });
registry.panel({ id: "testrun", title: "Test run", icon: TestRunIcon, order: 2, render: TestRunSide, main: TestRunMain });
registry.panel({ id: "env", title: "Environments", icon: EnvIcon, order: 3, render: EnvSide, main: EnvMain });
registry.panel({ id: "contract", title: "Contract & mock", icon: ContractIcon, order: 4, render: ContractPanel });
registry.panel({ id: "agents", title: "AI agents", icon: AgentsIcon, order: 5, render: AgentsPanel });
registry.panel({ id: "settings", title: "Settings", icon: SettingsIcon, order: 9, render: SettingsSide, main: SettingsMain, wide: true });

registry.slot("files.bottom", { id: "git.changes", order: 0, render: ChangesCard });
registry.slot("app.overlays", { id: "cookies.jar", order: 0, render: CookieJarDialog });

registry.command({ id: "env.open", title: "Open the environments", group: "Panels", run: () => useUI.getState().setPanel("env") });
registry.command({ id: "testrun.open", title: "Open the test run", group: "Panels", run: () => useUI.getState().setPanel("testrun") });
registry.command({
  id: "settings.open",
  title: "Open the settings",
  group: "Panels",
  run: (section) => {
    useUI.getState().setPanel("settings");
    if (typeof section === "string") requestAnimationFrame(() => goToSection(section));
  },
});
registry.command({ id: "cookies.openJar", title: "Open the cookie jar", group: "Panels", run: () => useCookieJar.getState().setOpen(true) });
