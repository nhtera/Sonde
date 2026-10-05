// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import defaultMdxComponents from "fumadocs-ui/mdx";
import type { MDXComponents } from "mdx/types";
import type { ComponentProps } from "react";
import { strings } from "@/content/strings";
import { CodeBlock } from "./docs/code-block";

// Scrollable code and tables are keyboard-focusable named groups (see
// docs/code-block.tsx for why code does not use Fumadocs' CodeBlock).
const group = (label: string) => ({ role: "group", "aria-label": label, tabIndex: 0 });

export function getMDXComponents(components?: MDXComponents) {
  return {
    ...defaultMdxComponents,
    pre: (props: ComponentProps<"pre">) => <CodeBlock {...props} />,
    table: (props: ComponentProps<"table">) => (
      <div className="relative overflow-auto prose-no-margin my-6" {...group(strings.docs.tableLabel)}>
        <table {...props} />
      </div>
    ),
    ...components,
  } satisfies MDXComponents;
}
