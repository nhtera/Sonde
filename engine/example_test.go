// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/nhtera/sonde/engine"
)

// Run a request file against a local server and inspect the result.
func Example() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status": "ok", "items": [1, 2, 3]}`)
	}))
	defer srv.Close()

	dir, _ := os.MkdirTemp("", "example")
	defer os.RemoveAll(dir) //nolint:errcheck // example cleanup
	src := []byte(`GET {{base}}/health
HTTP 200
[Captures]
count: jsonpath "$.items" count
[Asserts]
jsonpath "$.status" == "ok"
`)
	r := engine.NewRunner(engine.Options{Variables: map[string]any{"base": srv.URL}})
	defer r.Close() //nolint:errcheck // example cleanup
	res, err := r.RunSource(context.Background(), filepath.Join(dir, "health.hurl"), src)
	if err != nil {
		fmt.Println(err)
		return
	}
	entry := res.Entries[0]
	fmt.Println("success:", res.Success)
	fmt.Println("status:", entry.Calls[0].Response.Status)
	fmt.Println("captured:", entry.Captures[0].Name, entry.Captures[0].Value)
	// Output:
	// success: true
	// status: 200
	// captured: count 3
}
