// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Command sonde runs and tests HTTP requests written in plain text.
package main

import (
	"os"

	"github.com/nhtera/sonde/internal/cli"
)

// main is the only place in the program allowed to call os.Exit.
func main() {
	os.Exit(cli.Execute())
}
