// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

/** Whether path is a request file (.hurl or .sonde): the editor parses,
 * runs and checks only these; others (data, secrets, YAML) are text. */
export const isRequestPath = (path: string | null | undefined): boolean => !!path && /\.(hurl|sonde)$/.test(path);
