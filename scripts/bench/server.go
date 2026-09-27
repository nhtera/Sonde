// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// server is a minimal HTTP server used only by scripts/bench.sh: it binds
// an ephemeral port (never 8000-8003, reserved for the conformance
// harness), answers every request with a small fixed JSON body, and prints
// "PORT=<n>" on its own line to stdout so the caller can read it back. The
// caller is responsible for killing it (e.g. `kill $PID` in a trap); it
// does not watch stdin, since a background job's stdin is often already at
// EOF, which would exit it immediately.
package main

import (
	"fmt"
	"io"
	"net"
	"net/http" //nolint:depguard // bench tooling only, outside the architecture's net/http boundary (§2); never linked into the sonde binary
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench server: listen:", err)
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("PORT=%d\n", port)
	_ = os.Stdout.Sync()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":1,"name":"bench"}`)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}
