// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package vars lists the variables a request file can use, for completion
// and the variables panel: its sonde.yaml environment's (found the way a
// run finds it), the session overrides, and the captures of its last run,
// each with where it comes from. Secret values are never listed.
package vars

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/runsvc"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/value"
)

// Sources, highest precedence first.
const (
	SourceCapture  = "capture"
	SourceOverride = "override"
	SourceProject  = "project" // sonde.yaml and its files
)

// Var is a variable a file can use.
type Var struct {
	Name string `json:"name"`
	// Display is the value as text; empty for a secret.
	Display string `json:"display"`
	Source  string `json:"source"`
	// Origin is the project file it comes from ("sonde.yaml", a variables
	// or secrets file), for a project variable.
	Origin string `json:"origin,omitempty"`
	Secret bool   `json:"secret"`
}

// Vars lists variables.
type Vars struct {
	project   func() *sandbox.Root
	captures  func(file string) []runsvc.Capture
	overrides func() map[string]string
}

// New returns the variables lister.
func New(project func() *sandbox.Root, captures func(string) []runsvc.Capture, overrides func() map[string]string) *Vars {
	return &Vars{project: project, captures: captures, overrides: overrides}
}

// For lists the variables of file (project-relative) in environment env
// ("" for the project's default), by name; a name defined by several
// sources is listed once, from the one a run uses.
func (v *Vars) For(file, env string) ([]Var, error) {
	root := v.project()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	byName := map[string]Var{}
	if p, err := projectOf(root, file); err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	} else if p != nil {
		if env == "" {
			env = p.Defaults.Env
		}
		if env != "" {
			names, _ := p.VariableNames(env)
			vals, _, _ := p.Resolve(env)
			for name, src := range names {
				it := Var{Name: name, Source: SourceProject, Origin: "sonde.yaml", Secret: src.Secret}
				if src.File != "" {
					it.Origin = src.File
				}
				if !src.Secret {
					if val, ok := vals[name]; ok {
						it.Display = value.Display(val)
					}
				}
				byName[name] = it
			}
		}
	}
	if v.overrides != nil {
		for name, text := range v.overrides() {
			byName[name] = Var{Name: name, Display: text, Source: SourceOverride}
		}
	}
	if v.captures != nil {
		for _, c := range v.captures(file) {
			byName[c.Name] = Var{Name: c.Name, Display: c.Value, Source: SourceCapture, Secret: c.Secret}
		}
	}
	out := make([]Var, 0, len(byName))
	for _, it := range byName {
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b Var) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// projectOf loads the sonde.yaml a run of file would use; nil when none.
func projectOf(root *sandbox.Root, file string) (*config.Project, error) {
	if file == "" || filepath.IsAbs(file) || strings.Contains(file, "..") {
		return nil, apperr.New(apperr.Denied, "not a path in the project: "+file)
	}
	dir := filepath.Dir(filepath.Join(root.Dir(), filepath.FromSlash(file)))
	path, ok, err := config.NewProjectCache().FindProject(dir)
	if err != nil || !ok {
		return nil, err
	}
	return config.LoadProject(path)
}

// Service is the variables bindings.
type Service struct{ v *Vars }

// NewService returns the bindings over v.
func NewService(v *Vars) *Service { return &Service{v: v} }

// For lists the variables of file in env.
func (s *Service) For(file, env string) ([]Var, error) { return s.v.For(file, env) }
