// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package docs

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed compat.md.tmpl
var compatTemplate string

// RenderCompat renders docs/compat.md from t using compat.md.tmpl. The
// result is deterministic for a given Table.
func RenderCompat(t *Table) ([]byte, error) {
	tmpl, err := template.New("compat.md").Parse(compatTemplate)
	if err != nil {
		return nil, fmt.Errorf("docs: parse compat.md.tmpl: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t); err != nil {
		return nil, fmt.Errorf("docs: render compat.md: %w", err)
	}
	return buf.Bytes(), nil
}
