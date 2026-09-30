// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Hover docs arrive as Markdown turned into HTML: keep text and formatting
// only (no scripts, frames, event handlers or links out of the page).

const allowed = new Set(["P", "CODE", "PRE", "EM", "STRONG", "B", "I", "UL", "OL", "LI", "BR", "HR", "SPAN", "DIV", "H1", "H2", "H3", "H4", "BLOCKQUOTE", "TABLE", "THEAD", "TBODY", "TR", "TH", "TD", "A"]);

export function sanitizeHTML(html: string): string {
  const doc = new DOMParser().parseFromString(`<div>${html}</div>`, "text/html");
  const root = doc.body.firstElementChild!;
  const walk = (el: Element) => {
    for (const child of [...el.children]) {
      if (!allowed.has(child.tagName)) {
        child.replaceWith(doc.createTextNode(child.textContent ?? ""));
        continue;
      }
      for (const attr of [...child.attributes]) {
        const keep = attr.name === "class" || (child.tagName === "A" && attr.name === "href" && /^https:\/\//.test(attr.value));
        if (!keep) child.removeAttribute(attr.name);
      }
      if (child.tagName === "A" && child.hasAttribute("href")) {
        child.setAttribute("target", "_blank");
        child.setAttribute("rel", "noreferrer noopener");
      }
      walk(child);
    }
  };
  walk(root);
  return root.innerHTML;
}
