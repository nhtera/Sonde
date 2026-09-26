// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build race

package lsp

// raceEnabled skips timing budgets, which the race detector's overhead
// makes meaningless.
const raceEnabled = true
