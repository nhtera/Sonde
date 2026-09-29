// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Command fixture-server serves shop-api on 127.0.0.1 for the desktop's
// browser tests and manual checks (see desktop/testdata/shop-api).
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/nhtera/sonde/desktop/internal/fixture"
)

func main() {
	port := flag.Int("port", 34120, "port on 127.0.0.1")
	flag.Parse()
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture-server:", err)
		os.Exit(1)
	}
	fmt.Printf("shop-api on http://%s\n", ln.Addr())
	srv := &http.Server{Handler: fixture.New(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, "fixture-server:", err)
		os.Exit(1)
	}
}
