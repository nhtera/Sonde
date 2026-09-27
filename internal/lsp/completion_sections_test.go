// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"testing"

	"github.com/nhtera/sonde/internal/docs"
	"github.com/nhtera/sonde/internal/syntax"
)

// TestOptionShapesMatchTable guards against optionShapes drifting from
// docs.Table.Options (an option added to one but not the other would get
// wrong or no value-hint completion).
func TestOptionShapesMatchTable(t *testing.T) {
	table, err := docs.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, e := range append(append([]docs.Entry(nil), table.Options...), table.Sonde.Options...) {
		want[e.Name] = true
	}
	got := map[string]bool{}
	for name := range optionShapes {
		got[name] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("docs.Table.Options has %q, optionShapes does not", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("optionShapes has %q, docs.Table.Options does not", name)
		}
	}
}

// sampleOptionValue is a value that parses for shape, used only to prove
// optionShapes still matches internal/syntax/parse_option.go's unexported
// optionShapes.
func sampleOptionValue(shape optionShape) string {
	switch shape {
	case shapeBoolean:
		return "true"
	case shapeNatural, shapeCount:
		return "3"
	case shapeDuration:
		return "500ms"
	case shapeVariable:
		return "token=value"
	case shapeVerbosity:
		return "brief"
	default: // shapeString, shapeFilename, shapeFilenamePassword
		return "example"
	}
}

// TestOptionShapeSamplesParse guards against optionShapes drifting from
// internal/syntax/parse_option.go's unexported optionShapes: a sample value
// for the shape this package believes an option has must actually parse.
func TestOptionShapeSamplesParse(t *testing.T) {
	for name, shape := range optionShapes {
		src := "GET http://x/\n[Options]\n" + name + ": " + sampleOptionValue(shape) + "\n"
		if _, err := syntax.Parse("<test>", []byte(src), syntax.DialectSonde); err != nil {
			t.Errorf("%s (shape %d): sample %q did not parse: %v", name, shape, sampleOptionValue(shape), err)
		}
	}
}
