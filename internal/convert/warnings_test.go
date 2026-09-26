// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"bytes"
	"strings"
	"testing"
)

func TestSummarizeDeterministic(t *testing.T) {
	out := Output{
		Warnings: []Warning{
			{Kind: "b-kind", Message: "second"},
			{Kind: "a-kind", Message: "zeta"},
			{Kind: "a-kind", Message: "alpha"},
		},
		Skipped: []Skipped{
			{Name: "z.hurl", Reason: "unsupported"},
			{Name: "a.hurl", Reason: "unsupported"},
		},
	}
	res := &Result{Files: []string{"a.hurl", "b.hurl"}, RequestCount: 2}

	var b1, b2 bytes.Buffer
	Summarize(&b1, out, res)
	Summarize(&b2, out, res)
	if b1.String() != b2.String() {
		t.Fatalf("Summarize is not deterministic:\n--- 1 ---\n%s\n--- 2 ---\n%s", b1.String(), b2.String())
	}
	got := b1.String()
	for _, want := range []string{
		"wrote 2 file(s), 2 request(s)", "a.hurl", "b.hurl",
		"a.hurl: unsupported", "z.hurl: unsupported",
		"a-kind:", "b-kind:", "alpha", "zeta", "second",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
	// Skipped items must be sorted by name (a.hurl before z.hurl).
	if strings.Index(got, "a.hurl: unsupported") > strings.Index(got, "z.hurl: unsupported") {
		t.Errorf("skipped items not sorted:\n%s", got)
	}
	// Warning kinds sorted (a-kind before b-kind), messages within a kind
	// sorted (alpha before zeta).
	if strings.Index(got, "a-kind:") > strings.Index(got, "b-kind:") {
		t.Errorf("warning kinds not sorted:\n%s", got)
	}
	if strings.Index(got, "alpha") > strings.Index(got, "zeta") {
		t.Errorf("warning messages not sorted within kind:\n%s", got)
	}
}

func TestSummarizeDryRunWording(t *testing.T) {
	var b bytes.Buffer
	Summarize(&b, Output{}, &Result{DryRun: true, Files: []string{"a.hurl"}})
	if !strings.Contains(b.String(), "would write") {
		t.Errorf("dry run summary should say \"would write\":\n%s", b.String())
	}
}

func TestSummarizeProject(t *testing.T) {
	var b bytes.Buffer
	Summarize(&b, Output{ProjectYAML: []byte("version: 1\n")}, &Result{Project: "sonde.yaml"})
	if !strings.Contains(b.String(), "sonde.yaml") {
		t.Errorf("summary should mention sonde.yaml:\n%s", b.String())
	}

	b.Reset()
	Summarize(&b, Output{ProjectYAML: []byte("version: 1\n")}, &Result{ProjectSkipped: true})
	if !strings.Contains(b.String(), "already exists") {
		t.Errorf("summary should say sonde.yaml already exists:\n%s", b.String())
	}
}

func TestSummarizeQuotesControlCharacters(t *testing.T) {
	out := Output{
		Skipped:  []Skipped{{Name: "evil\x1b[2J", Reason: "multi\nline"}},
		Warnings: []Warning{{Kind: WarnScript, Message: "bell\a"}},
	}
	var b bytes.Buffer
	Summarize(&b, out, &Result{})
	got := b.String()
	if strings.ContainsAny(got, "\x1b\a") || !strings.Contains(got, `evil\x1b[2J: multi\nline`) || !strings.Contains(got, `bell\a`) {
		t.Errorf("summary:\n%q", got)
	}
}
