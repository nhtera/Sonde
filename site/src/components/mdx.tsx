// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import defaultMdxComponents from "fumadocs-ui/mdx";
import type { MDXComponents } from "mdx/types";

export function getMDXComponents(components?: MDXComponents) {
  return { ...defaultMdxComponents, ...components } satisfies MDXComponents;
}
