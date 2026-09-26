// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"
	"sort"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
)

// resolvedVars is one scope's variables, folded to a name->value map (the
// "default" sonde.yaml environment, or one named environment) plus the
// secret names declared in that scope, in first-seen order (mapping doc,
// "Variables and environments").
type resolvedVars struct {
	values      map[string]string
	secretNames []string
}

// mergeVariableLayers folds layers (root-first: collection, then each
// enclosing folder, then the request itself, or one named environment's
// own list as a single layer) into one resolvedVars: a later layer's value
// for the same name replaces an earlier one, and a secret name is kept
// once regardless of how many layers declare it.
func mergeVariableLayers(layers [][]variable) (resolvedVars, []convert.Warning) {
	out := resolvedVars{values: map[string]string{}}
	seenSecret := map[string]bool{}
	var warns []convert.Warning
	for _, layer := range layers {
		for _, v := range layer {
			if v.Disabled || v.Name == "" {
				continue
			}
			name := convert.VariableName(v.Name)
			if v.Secret {
				if !seenSecret[name] {
					seenSecret[name] = true
					out.secretNames = append(out.secretNames, name)
				}
				continue
			}
			value, w := resolveValue(v)
			warns = append(warns, w...)
			out.values[name] = value
		}
	}
	return out, warns
}

// resolveValue reads a Variable's "value" field, which the schema shapes
// three ways (mapping doc, "Variables and environments"): a plain string,
// a {type, data} object (its data is used), or a variant array (the
// Selected entry, or the first).
func resolveValue(v variable) (string, []convert.Warning) {
	if v.Value.Kind == 0 {
		return "", nil
	}
	var s string
	if v.Value.Decode(&s) == nil {
		return s, nil
	}
	var typed struct {
		Data string `yaml:"data"`
	}
	if v.Value.Decode(&typed) == nil && typed.Data != "" {
		return typed.Data, nil
	}
	var variants []struct {
		Title    string `yaml:"title"`
		Selected bool   `yaml:"selected"`
		Value    string `yaml:"value"`
	}
	if v.Value.Decode(&variants) == nil && len(variants) > 0 {
		chosen := variants[0]
		for _, vv := range variants {
			if vv.Selected {
				chosen = vv
				break
			}
		}
		var warns []convert.Warning
		if len(variants) > 1 {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("variable %q: only the %q value variant is imported, %d other(s) dropped", v.Name, chosen.Title, len(variants)-1)})
		}
		return chosen.Value, warns
	}
	return "", nil
}

// seedWith folds base's values and secret names under own's: own's own
// declaration of a name — plain or secret — always wins, but a name only
// base declares still reaches this environment (mapping doc, "Variables
// and environments": a non-default environment is seeded with the
// collection/default layer, the way Postman's importer already does).
func seedWith(base, own resolvedVars) resolvedVars {
	out := resolvedVars{values: make(map[string]string, len(base.values)+len(own.values))}
	secret := make(map[string]bool, len(base.secretNames)+len(own.secretNames))
	for k, v := range base.values {
		out.values[k] = v
	}
	for _, n := range base.secretNames {
		secret[n] = true
	}
	for k, v := range own.values {
		out.values[k] = v
		delete(secret, k)
	}
	for _, n := range own.secretNames {
		delete(out.values, n)
		secret[n] = true
	}
	if len(secret) > 0 {
		out.secretNames = make([]string, 0, len(secret))
		for n := range secret {
			out.secretNames = append(out.secretNames, n)
		}
		sort.Strings(out.secretNames)
	}
	return out
}

// defaultEnvName is the sonde.yaml environment name this importer always
// generates from the collection/folder/request variable layers
// (mapping doc, "Variables and environments"); it is reserved, so a
// same-named OpenCollection environment never overwrites it (M8).
const defaultEnvName = "default"

// uniqueEnvName returns name, or name with a " (2)", " (3)", ... suffix if
// it collides with a name already in used; the result is added to used.
func uniqueEnvName(name string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", name, n)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// buildProject renders the sonde.yaml skeleton and the secrets stubs for a
// "default" environment (defaultLayers: collection/folder/request
// variables merged, mapping doc "Variables and environments") plus one
// sonde.yaml environment per doc.Config.Environments entry, each seeded
// with the default layer (seedWith) and reserved against a name collision
// with "default" or another environment (uniqueEnvName).
func buildProject(doc *document, defaultLayers [][]variable) ([]byte, []convert.RawFile, []convert.Warning) {
	skeleton := config.ProjectSkeleton{Environments: map[string]config.EnvironmentSkeleton{}}
	var extra []convert.RawFile
	var warns []convert.Warning
	stubUsed := map[string]bool{}

	def, w := mergeVariableLayers(defaultLayers)
	warns = append(warns, w...)
	defEnv := config.EnvironmentSkeleton{Variables: def.values}
	if len(def.secretNames) > 0 {
		path := convert.StubPath(defaultEnvName, stubUsed)
		extra = append(extra, secretsStub(path, def.secretNames))
		defEnv.SecretsFiles = []string{path}
		warns = append(warns, secretWarnings(defaultEnvName, def.secretNames)...)
	}
	skeleton.Environments[defaultEnvName] = defEnv
	skeleton.DefaultEnv = defaultEnvName

	envNamesUsed := map[string]bool{defaultEnvName: true}
	for _, env := range doc.Config.Environments {
		if env.Name == "" {
			continue
		}
		warns = append(warns, environmentFieldWarnings(env)...)
		name := uniqueEnvName(env.Name, envNamesUsed)
		if name != env.Name {
			warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("environment %q collides with the generated %q environment or another environment; renamed to %q", env.Name, defaultEnvName, name)})
		}
		rv, w := mergeVariableLayers([][]variable{env.Variables})
		warns = append(warns, w...)
		rv = seedWith(def, rv)
		es := config.EnvironmentSkeleton{Variables: rv.values}
		if len(rv.secretNames) > 0 {
			path := convert.StubPath(name, stubUsed)
			extra = append(extra, secretsStub(path, rv.secretNames))
			es.SecretsFiles = []string{path}
			warns = append(warns, secretWarnings(name, rv.secretNames)...)
		}
		skeleton.Environments[name] = es
	}
	warns = append(warns, configFieldWarnings(doc.Config)...)

	out, err := config.EmitProject(skeleton)
	if err != nil {
		// EmitProject only fails on a YAML encoding error, which the
		// skeleton built above (plain strings and maps) cannot trigger.
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported, Message: "sonde.yaml: " + err.Error()})
		return nil, extra, warns
	}
	return out, extra, warns
}

// environmentFieldWarnings names env's fields with no Sonde equivalent
// (mapping doc, "Variables and environments"): none of them is enforced
// or applied, only detected, so a mistyped value never fails the import.
func environmentFieldWarnings(env environment) []convert.Warning {
	var warns []convert.Warning
	if env.ExternalSecrets.Kind != 0 {
		warns = append(warns, convert.Warning{Kind: convert.WarnSecret,
			Message: fmt.Sprintf("environment %q: externalSecrets has no Sonde equivalent (Sonde does not call out to a secrets manager); its variables contribute no value or stub entry", env.Name)})
	}
	if env.DotEnvFilePath != "" {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
			Message: fmt.Sprintf("environment %q: dotEnvFilePath has no Sonde equivalent", env.Name)})
	}
	if env.Extends != "" {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
			Message: fmt.Sprintf("environment %q: extends has no Sonde equivalent; environments are not merged", env.Name)})
	}
	if env.ClientCertificates.Kind != 0 {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported,
			Message: fmt.Sprintf("environment %q: clientCertificates has no Sonde equivalent", env.Name)})
	}
	return warns
}

// configFieldWarnings names doc.Config's collection-wide fields with no
// Sonde equivalent (mapping doc, "Unsupported entirely").
func configFieldWarnings(c collConfig) []convert.Warning {
	var warns []convert.Warning
	if c.Proxy.Kind != 0 {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported, Message: "config.proxy has no Sonde equivalent"})
	}
	if c.Protobuf.Kind != 0 {
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupported, Message: "config.protobuf has no Sonde equivalent"})
	}
	return warns
}

func secretsStub(path string, names []string) convert.RawFile {
	vars := make(map[string]string, len(names))
	for _, n := range names {
		vars[n] = ""
	}
	return convert.RawFile{Path: path, Data: config.EmitVariables(vars), Keep: true}
}

func secretWarnings(scope string, names []string) []convert.Warning {
	warns := make([]convert.Warning, len(names))
	for i, n := range names {
		warns[i] = convert.Warning{Kind: convert.WarnSecret,
			Message: fmt.Sprintf("%s: secret variable %q has no value in the output; fill it in the secrets stub", scope, n)}
	}
	return warns
}
