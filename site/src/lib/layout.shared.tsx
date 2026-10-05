// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { GithubLogoIcon } from "@phosphor-icons/react/ssr";
import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { Logo } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { strings } from "@/content/strings";
import { REPO_URL } from "./urls";

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      title: (
        <>
          <Logo size={22} />
          <span>{strings.site.name}</span>
        </>
      ),
    },
    links: [
      { type: "icon", url: REPO_URL, text: "GitHub", label: "GitHub", icon: <GithubLogoIcon />, external: true },
    ],
    slots: { themeSwitch: ThemeToggle },
  };
}
