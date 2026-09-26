// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
)

// TestBuildEntryWebSocketSkipped checks M5: a JetBrains WEBSOCKET request
// has no Sonde equivalent and is skipped, not turned into a broken
// "GET WEBSOCKET url" entry.
func TestBuildEntryWebSocketSkipped(t *testing.T) {
	r := &request{method: "WEBSOCKET", url: "wss://a.test/socket"}
	_, _, skip := buildEntry(r)
	if skip == "" {
		t.Fatal("expected a skip reason")
	}
}

// TestBuildEntryGraphQLMappedToPost checks M5: a JetBrains GRAPHQL request
// is sent as a POST.
func TestBuildEntryGraphQLMappedToPost(t *testing.T) {
	r := &request{method: "GRAPHQL", url: "https://a.test/graphql"}
	spec, _, skip := buildEntry(r)
	if skip != "" {
		t.Fatalf("skip = %q", skip)
	}
	if spec.Method != "POST" {
		t.Errorf("method = %q, want POST", spec.Method)
	}
}

// TestBuildEntryCustomMethod checks M5: any uppercase method token (not
// just the fixed HTTP method list) is accepted as is.
func TestBuildEntryCustomMethod(t *testing.T) {
	r := &request{method: "PROPFIND", url: "https://a.test/dav"}
	spec, _, skip := buildEntry(r)
	if skip != "" {
		t.Fatalf("skip = %q", skip)
	}
	if spec.Method != "PROPFIND" {
		t.Errorf("method = %q, want PROPFIND", spec.Method)
	}
}

// TestBuildEntryOutputOverwriteWarning checks M16: JetBrains' plain ">>"
// never overwrites (it adds a numeric suffix instead), so mapping it to
// Sonde's always-overwriting "output:" warns; ">>!" does not.
func TestBuildEntryOutputOverwriteWarning(t *testing.T) {
	r := &request{method: "GET", url: "https://a.test/x", outputFile: &outputRedirect{path: "./out.json"}}
	_, warns, skip := buildEntry(r)
	if skip != "" {
		t.Fatalf("skip = %q", skip)
	}
	found := false
	for _, w := range warns {
		if w.Kind == convert.WarnUnsupportedOption && strings.Contains(w.Message, "overwrite") {
			found = true
		}
	}
	if !found {
		t.Errorf(">> should warn about overwrite semantics: %v", warns)
	}

	r2 := &request{method: "GET", url: "https://a.test/x", outputFile: &outputRedirect{path: "./out.json", overwrite: true}}
	_, warns2, _ := buildEntry(r2)
	for _, w := range warns2 {
		if strings.Contains(w.Message, "overwrite") {
			t.Errorf(">>! should not warn about overwrite semantics: %v", warns2)
		}
	}
}
