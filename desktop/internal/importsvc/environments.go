// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/convert"
)

// mergeEnvironments plans the converter's environments into the
// project's sonde.yaml name (at the project folder), for a project that
// has one: each goes into the environment into maps it to (its variables
// and secret names that environment does not define yet), or, unmapped,
// is added as an environment of its own name (one the project has is
// left as it is). Secret names are listed in "secrets:", their values
// left to each person's secrets file (kept out of git): no stub is
// written. The folder imported into gets no sonde.yaml: a run reads the
// nearest one, which would hide the project's environments.
func (p *plan) mergeEnvironments(name string, into map[string]string) error {
	var sk struct {
		Environments map[string]struct {
			Variables    map[string]string `yaml:"variables"`
			SecretsFiles []string          `yaml:"secrets_files"`
		} `yaml:"environments"`
		OpenAPI struct {
			Spec string `yaml:"spec"`
		} `yaml:"openapi"`
	}
	if err := yaml.Unmarshal(p.out.ProjectYAML, &sk); err != nil {
		return fmt.Errorf("convert: the environments do not read: %w", err)
	}
	if sk.OpenAPI.Spec != "" {
		p.notes = append(p.notes, fmt.Sprintf("%s keeps its own openapi: to check responses against %s, set openapi.spec to it by hand", name, path.Join(p.rel, sk.OpenAPI.Spec)))
	}
	stubs := map[string][]byte{}
	for _, x := range p.out.Extra {
		stubs[x.Path] = x.Data
	}
	p.out.Extra = nil
	// The collection's own environment last: an environment file's values
	// win over it where both go into the same environment.
	names := make([]string, 0, len(sk.Environments))
	for n := range sk.Environments {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if (a == "collection") != (b == "collection") {
			if a == "collection" {
				return 1
			}
			return -1
		}
		return strings.Compare(a, b)
	})
	p.importEnvs = slices.Sorted(slices.Values(names))
	if len(names) == 0 {
		return nil
	}

	notAdded := func(why string) {
		p.notes = append(p.notes, fmt.Sprintf("%s: %s not added to it", why, environments(p.importEnvs)))
	}
	switch fi, err := p.root.Lstat(name); {
	case err != nil:
		notAdded(name + " does not read")
		return nil
	case fi.Mode()&fs.ModeSymlink != 0:
		notAdded(name + " is a symbolic link, edited by hand")
		return nil
	}
	abs, err := p.root.Path(name) //nolint:forbidigo // read only: the edit is written through the Root
	if err != nil {
		return err
	}
	proj, err := config.LoadProject(abs)
	if err != nil {
		notAdded(name + " does not load")
		return nil
	}
	var merges []config.EnvMerge
	add := map[string]config.EnvironmentSkeleton{}
	for _, n := range names {
		env := sk.Environments[n]
		var secrets []string
		for _, f := range env.SecretsFiles {
			assigns, _ := config.ParseProperties(stubs[f], config.Forced)
			for _, a := range assigns {
				secrets = append(secrets, a.Name)
			}
		}
		switch target := into[n]; {
		case target != "":
			if _, ok := proj.Environments[target]; !ok {
				return apperr.New(apperr.Invalid, fmt.Sprintf("%s has no environment %s to import %s into", name, target, n))
			}
			merges = append(merges, config.EnvMerge{Env: target, Variables: env.Variables, Secrets: secrets})
			p.secretNames += len(secrets)
		default:
			if _, ok := proj.Environments[n]; ok {
				p.envsKept = append(p.envsKept, n)
				continue
			}
			add[n] = config.EnvironmentSkeleton{Variables: env.Variables, Secrets: secrets}
			p.secretNames += len(secrets)
		}
	}
	edits, kept, err := proj.MergeEnvironments(merges, add)
	if err != nil {
		if errors.Is(err, config.ErrEditByHand) {
			notAdded(name + " is in a shape edited by hand")
		} else {
			notAdded(fmt.Sprintf("%s is not edited (%v)", name, err))
		}
		return nil
	}
	for _, env := range slices.Sorted(maps.Keys(kept)) {
		p.notes = append(p.notes, fmt.Sprintf("%s has %s already in %s: kept as written", env, strings.Join(kept[env], ", "), name))
	}
	perm := fs.FileMode(0o644)
	if fi, err := p.root.Stat(name); err == nil {
		perm = fi.Mode().Perm()
	}
	for _, e := range edits {
		if e.Path == abs {
			p.files = append(p.files, convert.PlannedFile{Path: name, Data: e.Data, Perm: perm})
			p.edited = name
		}
	}
	return nil
}

// environments names environments in a note: "the environment a is" or
// "the environments a, b are".
func environments(names []string) string {
	if len(names) == 1 {
		return "the environment " + names[0] + " is"
	}
	return "the environments " + strings.Join(names, ", ") + " are"
}

// projectFile returns the name of the project's sonde.yaml (at the
// project folder), "" when it has none.
func (p *plan) projectFile() string {
	for _, name := range []string{"sonde.yaml", "sonde.yml"} {
		if _, err := p.root.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			return name
		}
	}
	return ""
}
