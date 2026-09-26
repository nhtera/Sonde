// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package opencollection imports a Bruno OpenCollection YAML collection —
// a single file or a directory — into Sonde request files, through the
// shared internal/convert writer. The version supported and the full
// field mapping are pinned in docs/decisions/0002-opencollection-mapping.md;
// this package implements that mapping and nothing else.
package opencollection

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// ImportFile converts data, one OpenCollection YAML document (the single-
// file layout, docs/decisions/0002-opencollection-mapping.md), to a
// convert.Output. dialect is the dialect of the generated files
// (convert.Options.Dialect()). data is capped at maxFileSize (16 MiB), the
// same per-file cap the directory layout applies to each of its files,
// even though the CLI itself may have read up to convert.MaxInput (64
// MiB) getting it here.
func ImportFile(data []byte, dialect syntax.Dialect) (convert.Output, error) {
	if len(data) > maxFileSize {
		return convert.Output{}, fmt.Errorf("opencollection: input larger than %d MiB", maxFileSize>>20)
	}
	doc, warns, err := parseDocument(data)
	if err != nil {
		return convert.Output{}, err
	}
	return assemble(doc, dialect, warns), nil
}

// ImportDir converts the OpenCollection directory at dir (the directory
// layout, docs/decisions/0002-opencollection-mapping.md) to a
// convert.Output. Every read is confined inside dir through os.Root, so a
// symlink placed in the collection cannot make the import read outside it.
func ImportDir(dir string, dialect syntax.Dialect) (convert.Output, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return convert.Output{}, fmt.Errorf("opencollection: %w", err)
	}
	defer func() { _ = root.Close() }()

	d := &dirLoad{budget: newBudget()}
	doc, err := loadDirectory(root.FS(), d)
	if err != nil {
		return convert.Output{}, err
	}
	out := assemble(doc, dialect, d.warnings)
	out.Skipped = append(out.Skipped, d.skipped...)
	return out, nil
}

// parseDocument decodes data as one OpenCollection document, rejecting a
// second "---"-separated document (like sonde.yaml, docs/sonde-yaml.md)
// but never a version it does not recognize: an unpinned major.minor
// version only warns (docs/decisions/0002-opencollection-mapping.md,
// "Pinned version").
func parseDocument(data []byte) (*document, []convert.Warning, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		return nil, nil, fmt.Errorf("opencollection: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, nil, fmt.Errorf("opencollection: input has more than one YAML document")
	}
	doc, err := decodeDocumentNode(&root)
	if err != nil {
		return nil, nil, fmt.Errorf("opencollection: %w", err)
	}
	var warns []convert.Warning
	if doc.OpenCollection != "" && !strings.HasPrefix(doc.OpenCollection, "1.") {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
			Message: fmt.Sprintf("unrecognized opencollection version %q, importing as 1.x best-effort", doc.OpenCollection)})
	}
	return doc, warns, nil
}

// assemble walks doc's item tree into request files and folds every scope's
// variables into the sonde.yaml skeleton and its secrets stubs.
func assemble(doc *document, dialect syntax.Dialect, warns []convert.Warning) convert.Output {
	warns = append(warns, decodeWarnings(doc.decodeErrors)...)
	wr := &walkResult{dialect: dialect}
	wr.varLayers = append(wr.varLayers, doc.Request.Variables)
	root := ancestorState{}.extend(doc.Request)
	wr.walkItems(doc.Items, root, nil)

	project, extra, pwarns := buildProject(doc, wr.varLayers)

	return convert.Output{
		Files:       wr.files,
		Extra:       extra,
		ProjectYAML: project,
		Warnings:    append(append(warns, wr.warnings...), pwarns...),
		Skipped:     wr.skipped,
	}
}
