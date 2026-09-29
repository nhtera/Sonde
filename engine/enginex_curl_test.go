// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/enginex"
)

// TestRevealCurl renders BasicAuth credentials and a secret header in
// clear only on a runner set with RevealCurl.
func TestRevealCurl(t *testing.T) {
	const pw, key = "pw-sentinel-9", "key-sentinel-4" //nolint:gosec // G101: test sentinels
	src := []byte("GET http://localhost/\nX-Key: {{key}}\n[BasicAuth]\nbob: {{pw}}\n")
	secrets := map[string]string{"pw": pw, "key": key}
	render := func(reveal bool) string {
		r := NewRunner(Options{Secrets: secrets})
		if reveal {
			enginex.RevealCurl(r)
		}
		entries, err := r.RenderCurl(context.Background(), "t.hurl", src)
		if err != nil || len(entries) != 1 || entries[0].Err != nil {
			t.Fatalf("%v %+v", err, entries)
		}
		return entries[0].Command
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("bob:" + pw))
	masked, revealed := render(false), render(true)
	for _, s := range []string{pw, key, b64} {
		if strings.Contains(masked, s) {
			t.Errorf("default render shows %q:\n%s", s, masked)
		}
	}
	if !strings.Contains(revealed, key) || !strings.Contains(revealed, pw) && !strings.Contains(revealed, b64) {
		t.Errorf("revealed render hides a secret:\n%s", revealed)
	}
	if strings.Contains(revealed, "***") {
		t.Errorf("revealed render masks:\n%s", revealed)
	}
}
