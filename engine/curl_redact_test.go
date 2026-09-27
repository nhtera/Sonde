// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestRenderCurlRedactsEverywhere renders a secret in every place a request
// carries a value, through each quoting the curl renderer picks, and checks
// that no fragment of it survives redaction.
func TestRenderCurlRedactsEverywhere(t *testing.T) {
	secrets := []string{
		"sk_live-5.1~abcXYZ",
		`zzQ"zz'q\z`,
		"tab\tand\nnewline_zz",
		"<html&amp>zz",
	}
	positions := map[string]string{
		"url":       "GET http://localhost/{{pw}}\n",
		"query":     "GET http://localhost/\n[Query]\nk: {{pw}}\n",
		"header":    "GET http://localhost/\nX-Key: {{pw}}\n",
		"form":      "POST http://localhost/\n[Form]\nf: {{pw}}\n",
		"multipart": "POST http://localhost/\n[Multipart]\nf: {{pw}}\n",
		"cookie":    "GET http://localhost/\n[Cookies]\nc: {{pw}}\n",
		"user":      "GET http://localhost/\n[Options]\nuser: bob:{{pw}}\n",
		"basic":     "GET http://localhost/\n[BasicAuth]\nbob: {{pw}}\n",
		"json":      "POST http://localhost/\n{\"a\": \"{{pw}}\"}\n",
		"json-ml":   "POST http://localhost/\n{\n  \"a\": \"x{{pw}}y\"\n}\n",
		"text":      "POST http://localhost/\n```\nv={{pw}}\n```\n",
		"oneline":   "POST http://localhost/\n`v={{pw}}`\n",
	}
	for name, src := range positions {
		for _, secret := range secrets {
			f := parseEntries(t, src)
			entries, err := NewRunner(Options{Secrets: map[string]string{"pw": secret}}).RenderCurl(context.Background(), "test.hurl", f)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(entries) != 1 || entries[0].Err != nil {
				t.Fatalf("%s: %+v", name, entries)
			}
			cmd := entries[0].Command
			for _, frag := range []string{"zz", "abcXYZ", "live"} {
				if strings.Contains(secret, frag) && strings.Contains(cmd, frag) {
					t.Errorf("%s, secret %q: %q survives in\n%s", name, secret, frag, cmd)
				}
			}
		}
	}
}
