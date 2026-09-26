// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package httpfile imports ".http" files, the plain-text request format
// shared (with minor dialect differences) by the JetBrains HTTP Client and
// the VS Code REST Client extension, into Sonde request files. Every request
// of the input becomes one entry of a single output file; pre-request and
// response handler scripts are never executed, only kept as comments.
package httpfile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// Options are the .http-specific import inputs, on top of the shared
// convert.Options.
type Options struct {
	// EnvFiles are --env-file paths: JetBrains http-client.env.json
	// documents, each naming one or more environments. A sibling
	// "*.private.env.json" next to an entry (the JetBrains convention) is
	// read too, when present; its values are never written, only their
	// names, as a secrets stub.
	EnvFiles []string
}

// Import converts one .http file's data into a convert.Output. stem names
// the single output file (the input's base name without its extension, or
// "requests" for stdin); d is the dialect of the generated files.
func Import(stem string, data []byte, d syntax.Dialect, opts Options) (convert.Output, error) {
	doc := parseDocument(data)
	if len(doc.requests) == 0 {
		return convert.Output{}, fmt.Errorf("httpfile: no request found in %s", stem)
	}

	var out convert.Output
	order, raw := fileVarOrder(doc.fileVars)
	resolved, warns := resolveVars(order, raw, "file variable")
	out.Warnings = append(out.Warnings, warns...)
	for _, fv := range doc.fileVars {
		if fv.prompt {
			out.Warnings = append(out.Warnings, convert.Warning{
				Kind: convert.WarnUnsupportedOption,
				Message: fmt.Sprintf("@prompt %s: no interactive prompt at import time; set the %s variable yourself",
					fv.name, convert.VariableName(fv.name)),
			})
		}
	}

	// Each request is built, and validated through BuildFile, on its own:
	// one bad request (an invalid body, say) is skipped rather than
	// aborting the whole import.
	var goodSpecs []syntax.EntrySpec
	for _, r := range doc.requests {
		spec, rw, skip := buildEntry(r)
		if skip != "" {
			out.Skipped = append(out.Skipped, convert.Skipped{Name: entryLabel(r), Reason: skip})
			continue
		}
		if _, err := syntax.BuildFile([]syntax.EntrySpec{spec}, d); err != nil {
			out.Skipped = append(out.Skipped, convert.Skipped{Name: entryLabel(r), Reason: err.Error()})
			continue
		}
		out.Warnings = append(out.Warnings, rw...)
		goodSpecs = append(goodSpecs, spec)
	}
	if len(goodSpecs) == 0 {
		return convert.Output{}, fmt.Errorf("httpfile: no request could be converted in %s", stem)
	}
	f, err := syntax.BuildFile(goodSpecs, d)
	if err != nil {
		return convert.Output{}, err
	}
	out.Files = []convert.GeneratedFile{{Path: stem, File: f}}

	envs := map[string]config.EnvironmentSkeleton{}
	if len(resolved) > 0 {
		vars, mw := mergeVarNames(nil, order, resolved)
		out.Warnings = append(out.Warnings, mw...)
		envs["default"] = config.EnvironmentSkeleton{Variables: vars}
	}
	envWarns, err := applyEnvFiles(envs, opts.EnvFiles, &out, order, raw)
	if err != nil {
		return convert.Output{}, err
	}
	out.Warnings = append(out.Warnings, envWarns...)

	if len(envs) > 0 {
		skeleton := config.ProjectSkeleton{Environments: envs}
		if _, ok := envs["default"]; ok {
			skeleton.DefaultEnv = "default"
		}
		pj, err := config.EmitProject(skeleton)
		if err != nil {
			return convert.Output{}, err
		}
		out.ProjectYAML = pj
	}
	return out, nil
}

// StemFor derives the output file stem from an importer's INPUT argument:
// the base name without its extension, or "requests" for stdin ("-").
func StemFor(input string) string {
	if input == "-" {
		return "requests"
	}
	base := filepath.Base(input)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		return "requests"
	}
	return stem
}

// mergeVarNames adds every name in order into existing (creating it if
// nil), naming each with convert.VariableName so it matches the {{name}}
// placeholders ParseText produces from the same raw name, and looking its
// value up in resolved. Names are applied in order, for determinism (map
// iteration order is not); two different names that both sanitize to the
// same VariableName (e.g. "a.b" and "a_b") collide, in which case the
// later one in order wins and a warning names both.
func mergeVarNames(existing map[string]string, order []string, resolved map[string]string) (map[string]string, []convert.Warning) {
	if existing == nil {
		existing = make(map[string]string, len(order))
	}
	var warns []convert.Warning
	firstRaw := map[string]string{} // VariableName -> first raw name seen this call
	for _, name := range order {
		vn := convert.VariableName(name)
		if prev, ok := firstRaw[vn]; ok && prev != name {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("variables %q and %q both become {{%s}}; %q's value is used", prev, name, vn, name)})
		}
		firstRaw[vn] = name
		existing[vn] = resolved[name]
	}
	return existing, warns
}

// sortedKeys returns m's keys, sorted, for deterministic iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
