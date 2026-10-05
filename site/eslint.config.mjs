// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  js.configs.recommended,
  ...tseslint.configs.recommended,
  reactHooks.configs.flat["recommended-latest"],
  {
    files: ["src/**/*.{ts,tsx}", "*.ts"],
    languageOptions: {
      globals: { ...globals.browser },
    },
    rules: {
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
    },
  },
  {
    files: ["*.mjs", "*.config.ts", "scripts/**/*.mjs", "test/**/*.mjs", "src/**/*.test.ts"],
    languageOptions: {
      globals: { ...globals.node },
    },
  },
  {
    ignores: ["dist/**", "node_modules/**", ".source/**", ".output/**", ".tanstack/**", ".wrangler/**", ".cloudflare/**", "content/**", "src/routeTree.gen.ts", "deploy/node_modules/**"],
  },
);
