// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
)

// fileVarOrder flattens defs (in file order) to the declaration order of
// distinct names and a name -> raw value map; a later definition of the same
// name overrides an earlier one, matching how both dialects evaluate file
// variables top to bottom.
func fileVarOrder(defs []fileVarDef) (order []string, raw map[string]string) {
	raw = make(map[string]string, len(defs))
	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if !seen[d.name] {
			order = append(order, d.name)
			seen[d.name] = true
		}
		raw[d.name] = d.rawValue
	}
	return order, raw
}

// resolveVars flattens a set of name -> raw value definitions, substituting
// "{{other}}" references between them (in declaration order, with cycle
// protection): sonde.yaml variable values are literal, not
// template-expanded, so a value that itself references another variable
// (e.g. "@base = {{host}}/api") must be inlined at import time to keep its
// meaning. A dynamic variable ("{{$uuid}}") or a name defined nowhere in
// this set is left as a literal "{{...}}" placeholder for ParseText to
// handle at each request's own use site; label names the kind of value in
// warning messages ("file variable", "environment prod variable", ...).
func resolveVars(order []string, raw map[string]string, label string) (map[string]string, []convert.Warning) {
	resolved := make(map[string]string, len(order))
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(order))
	var warns []convert.Warning

	var resolve func(name string) string
	resolve = func(name string) string {
		if v, ok := resolved[name]; ok {
			return v
		}
		val, known := raw[name]
		if !known {
			return "{{" + name + "}}"
		}
		if state[name] == visiting {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("%s %q has a circular reference; its value is left unresolved", label, name)})
			return "{{" + name + "}}"
		}
		state[name] = visiting
		var b strings.Builder
		s := val
		for {
			open := strings.Index(s, "{{")
			if open < 0 {
				break
			}
			end := strings.Index(s[open+2:], "}}")
			if end < 0 {
				break
			}
			ref := strings.TrimSpace(s[open+2 : open+2+end])
			b.WriteString(s[:open])
			if ref == "" || strings.HasPrefix(ref, "$") {
				b.WriteString(s[open : open+2+end+2])
			} else {
				b.WriteString(resolve(ref))
			}
			s = s[open+2+end+2:]
		}
		b.WriteString(s)
		out := b.String()
		state[name] = done
		resolved[name] = out
		return out
	}
	for _, name := range order {
		resolve(name)
	}
	for _, name := range order {
		if strings.Contains(resolved[name], "{{") {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("%s %q resolves to %q, which sonde.yaml cannot expand further; set it directly or via --variable",
					label, name, resolved[name])})
		}
	}
	return resolved, warns
}

// applyEnvFiles reads every --env-file (a JetBrains http-client.env.json
// document) into envs, one sonde.yaml environment per top-level name other
// than the shared defaults "$shared". Each environment's own values (plus
// "$shared") are resolved together with the file's own "@var = value"
// variables (fileOrder/fileRaw), not against the file variables alone, so a
// file variable that itself references an environment-specific name (e.g.
// "@base = {{host}}/api" with "host" defined only in the env file) resolves
// to that environment's own value instead of staying a literal, unresolved
// "{{host}}/api" in every environment; the environment's own value wins on
// a name collision. Its sibling "*.private.env.json", if present, supplies
// a secrets stub for the same environments (values never written); a
// private "$shared" folds into every environment's stub, whether or not
// that environment has its own private entry.
func applyEnvFiles(envs map[string]config.EnvironmentSkeleton, files []string, out *convert.Output,
	fileOrder []string, fileRaw map[string]string) ([]convert.Warning, error) {
	var warns []convert.Warning
	stubUsed := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, ".private.env.json") {
			return nil, fmt.Errorf("httpfile: --env-file %s: a private env file holds secret values; "+
				"pass its public sibling instead (e.g. http-client.env.json), which reads this file automatically", path)
		}
		doc, err := readEnvFile(path)
		if err != nil {
			return nil, err
		}
		shared := doc["$shared"]

		privatePath := privateEnvPath(path)
		pdoc, err := readEnvFile(privatePath)
		privateExists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		var privateShared map[string]any
		if privateExists {
			privateShared = pdoc["$shared"]
		}

		names := map[string]bool{}
		for n := range doc {
			if n != "$shared" {
				names[n] = true
			}
		}
		if privateExists {
			for n := range pdoc {
				if n != "$shared" {
					names[n] = true
				}
			}
		}

		for _, name := range sortedKeys(names) {
			raw := make(map[string]string, len(shared)+len(doc[name]))
			var order []string
			for k, v := range shared {
				raw[k] = stringifyJSONValue(v)
				order = append(order, k)
			}
			for k, v := range doc[name] {
				if _, exists := raw[k]; !exists {
					order = append(order, k)
				}
				raw[k] = stringifyJSONValue(v)
			}
			sort.Strings(order)

			// Combine this environment's own raw values with the file's
			// own "@var = value" definitions, the environment winning on a
			// name collision, and resolve them together: a file variable
			// referencing an environment-specific name then resolves using
			// *this* environment's value for it, not just the file
			// variables' own (S2).
			combinedRaw := make(map[string]string, len(fileRaw)+len(raw))
			for _, n := range fileOrder {
				combinedRaw[n] = fileRaw[n]
			}
			for k, v := range raw {
				combinedRaw[k] = v
			}
			combinedOrder := make([]string, 0, len(fileOrder)+len(order))
			seenName := make(map[string]bool, len(fileOrder)+len(order))
			for _, n := range fileOrder {
				if !seenName[n] {
					seenName[n] = true
					combinedOrder = append(combinedOrder, n)
				}
			}
			for _, n := range order {
				if !seenName[n] {
					seenName[n] = true
					combinedOrder = append(combinedOrder, n)
				}
			}
			resolved, w := resolveVars(combinedOrder, combinedRaw, fmt.Sprintf("environment %s variable", name))
			warns = append(warns, w...)

			env := envs[name]
			env.Variables, w = mergeVarNames(env.Variables, combinedOrder, resolved)
			warns = append(warns, w...)
			envs[name] = env

			secretNames := map[string]bool{}
			if privateExists {
				for k := range pdoc[name] {
					secretNames[k] = true
				}
				for k := range privateShared {
					secretNames[k] = true
				}
			}
			if len(secretNames) == 0 {
				continue
			}
			stub := make(map[string]string, len(secretNames))
			for _, k := range sortedKeys(secretNames) {
				stub[convert.VariableName(k)] = ""
				warns = append(warns, convert.Warning{Kind: convert.WarnSecret,
					Message: fmt.Sprintf("environment %s: secret %s from a private env file is not written; fill its secrets stub", name, k)})
			}
			secretsPath := convert.StubPath(name, stubUsed)
			out.Extra = append(out.Extra, convert.RawFile{Path: secretsPath, Data: config.EmitVariables(stub), Keep: true})
			env = envs[name]
			env.SecretsFiles = append(env.SecretsFiles, secretsPath)
			envs[name] = env
		}
	}
	return warns, nil
}

// readEnvFile reads and decodes one JetBrains env-file document: a map from
// environment name to its variables, each a JSON scalar.
func readEnvFile(path string) (map[string]map[string]any, error) {
	data, err := convert.ReadInput(path, convert.MaxInput)
	if err != nil {
		return nil, err
	}
	var doc map[string]map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("httpfile: %s: %w", path, err)
	}
	return doc, nil
}

// privateEnvPath returns the sibling private env file JetBrains reads
// alongside path ("http-client.env.json" -> "http-client.private.env.json").
func privateEnvPath(path string) string {
	if rest, ok := strings.CutSuffix(path, ".env.json"); ok {
		return rest + ".private.env.json"
	}
	return path + ".private.env.json"
}

// stringifyJSONValue renders a decoded JSON scalar as the literal text a
// variables value would carry, matching internal/config's InferValue so the
// type it infers back from sonde.yaml is the one the env file declared.
func stringifyJSONValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(val)
	case string:
		return val
	case float64:
		if !math.IsInf(val, 0) && val == math.Trunc(val) && math.Abs(val) < 1e15 {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'g', -1, 64)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}
