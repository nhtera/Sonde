// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build server && e2eharness

// Command sonde-desktop-harness is a test-only host: the app over HTTP with
// a fixed token and a fixture project, for browser tests. It is never
// released.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nhtera/sonde/desktop/internal/serverauth"
)

// HarnessToken is the harness's fixed token, known to the browser tests.
const HarnessToken = "sonde-e2e-harness" //nolint:gosec // test-only, public by design

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sonde-desktop-harness:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("sonde-desktop-harness", flag.ContinueOnError)
	rootFlag := fs.String("root", "", "fixture project folder (required)")
	portFlag := fs.Int("port", 34115, "port on 127.0.0.1")
	data := fs.String("data", "", "app data folder (default: a new temporary folder)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rootFlag == "" {
		return fmt.Errorf("--root is required")
	}
	root, err := projectRoot(*rootFlag)
	if err != nil {
		return err
	}
	port, err := checkPort(*portFlag)
	if err != nil {
		return err
	}
	if *data == "" {
		if *data, err = os.MkdirTemp("", "sonde-harness-"); err != nil {
			return err
		}
		defer os.RemoveAll(*data)
	}
	dirs, err := openDirs(*data)
	if err != nil {
		return err
	}
	defer dirs.Close()
	guard := serverauth.NewFixed(port, HarnessToken)
	app := newServerApp(&Host{Mode: ModeHarness, Root: root, Dirs: dirs}, port, guard)
	go func() {
		if err := waitListening(port, 30*time.Second); err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("harness on http://%s:%d/ root %s\n", loopback, port, root)
	}()
	return app.Run()
}
