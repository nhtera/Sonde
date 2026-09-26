// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !race

package postman

// raceEnabled is whether this binary was built with -race, which slows
// execution enough that TestPostmanPerf's deadline does not apply.
const raceEnabled = false
