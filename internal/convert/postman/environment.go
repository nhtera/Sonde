// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
)

// envFile is a Postman environment export
// (https://schema.postman.com/collection/json/v2.1.0/, the sibling
// environment format): a name and a flat list of values.
type envFile struct {
	Name   string     `json:"name"`
	Values []envValue `json:"values"`
}

type envValue struct {
	Key     string     `json:"key"`
	Value   flexString `json:"value"`
	Type    string     `json:"type"`
	Enabled *bool      `json:"enabled"` // nil means enabled, matching a real export's default
}

func (v envValue) enabled() bool { return v.Enabled == nil || *v.Enabled }

// parseEnvironment reads one --environment file: its name (falling back to
// the file's own base name when the JSON has none) and its enabled values,
// split into plain variables and secret-typed names (never their values).
func (w *walker) parseEnvironment(ef EnvironmentFile) (name string, vars map[string]string, secretNames []string, err error) {
	var env envFile
	if err := json.Unmarshal(ef.Data, &env); err != nil {
		return "", nil, nil, fmt.Errorf("postman: --environment %s: invalid JSON: %w", ef.FileName, err)
	}
	name = strings.TrimSpace(env.Name)
	if name == "" {
		base := filepath.Base(ef.FileName)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	vars = map[string]string{}
	seen := map[string]string{}
	for _, v := range env.Values {
		if !v.enabled() {
			continue
		}
		key := convert.VariableName(v.Key)
		if prev, ok := seen[key]; ok && prev != v.Key {
			w.warn(convert.WarnUnsupported, fmt.Sprintf("%s: variable %q and %q both become %q; the later one wins", name, prev, v.Key, key))
		} else if !ok {
			seen[key] = v.Key
		}
		if strings.EqualFold(v.Type, "secret") {
			secretNames = append(secretNames, key)
			w.warn(convert.WarnSecret, fmt.Sprintf("%s: variable %q is a secret; add its value to the environment's secrets file", name, v.Key))
			continue
		}
		vars[key] = string(v.Value)
	}
	return name, vars, secretNames, nil
}

// uniqueEnvName sanitizes raw to a lowercase, hyphenated sonde.yaml
// environment name and de-duplicates it against used, appending "-2",
// "-3", ... as needed; used is updated with the result.
func uniqueEnvName(raw string, used map[string]bool) string {
	base := kebabName(raw)
	if base == "" {
		base = "environment"
	}
	if !used[base] {
		used[base] = true
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func kebabName(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// sortedSet returns the keys of set marked true, sorted, for a
// deterministic secrets stub.
func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k, v := range set {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ReadEnvironments reads Postman environment JSON files (--environment),
// each at most convert.MaxInput bytes.
func ReadEnvironments(paths []string) ([]EnvironmentFile, error) {
	envs := make([]EnvironmentFile, 0, len(paths))
	for _, path := range paths {
		raw, err := convert.ReadInput(path, convert.MaxInput)
		if err != nil {
			return nil, fmt.Errorf("--environment: %w", err)
		}
		envs = append(envs, EnvironmentFile{FileName: path, Data: raw})
	}
	return envs, nil
}
