// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { strings } from "@/content/strings";
import { REPO_URL } from "./urls";

export function baseOptions(): BaseLayoutProps {
  return {
    nav: { title: strings.site.name },
    githubUrl: REPO_URL,
  };
}
