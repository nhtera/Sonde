// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nhtera/sonde/internal/config"
)

// Listing bounds.
const (
	maxListFiles    = 2000
	maxListProjects = 100
	maxListDepth    = 16
)

type listInput struct {
	Dir string `json:"dir,omitempty" jsonschema:"a directory under the server root, relative to it (default: the root)"`
}

type listOutput struct {
	Files    []listedFile `json:"files"`
	Projects []project    `json:"projects"`
	// Truncated is set when the listing stopped at its limit.
	Truncated bool `json:"truncated,omitempty"`
}

type listedFile struct {
	Path    string `json:"path" jsonschema:"relative to the server root"`
	Dialect string `json:"dialect" jsonschema:"hurl or sonde"`
}

type project struct {
	Path         string   `json:"path" jsonschema:"the sonde.yaml file, relative to the server root"`
	Environments []string `json:"environments"`
	DefaultEnv   string   `json:"default_env,omitempty"`
	Error        string   `json:"error,omitempty" jsonschema:"why the file could not be loaded"`
}

// errListFull stops the walk at maxListFiles.
var errListFull = errors.New("listing full")

func (s *server) list(ctx context.Context, _ *sdk.CallToolRequest, in listInput) (*sdk.CallToolResult, listOutput, error) {
	start := time.Now()
	out := listOutput{Files: []listedFile{}, Projects: []project{}}
	dir := in.Dir
	if dir == "" {
		dir = "."
	}
	abs, rel, err := s.resolve(dir)
	if err != nil {
		s.audit("sonde_list", start, "dir=%q denied", in.Dir)
		return nil, out, err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		s.audit("sonde_list", start, "dir=%q not a directory", rel)
		return nil, out, errors.New(rel + ": not a directory")
	}
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable directory is skipped
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == abs {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			depth := strings.Count(strings.TrimPrefix(path, abs), string(filepath.Separator))
			if strings.HasPrefix(name, ".") || depth > maxListDepth {
				return filepath.SkipDir
			}
			return nil
		}
		// Regular files only: a symbolic link is not followed.
		if !d.Type().IsRegular() {
			return nil
		}
		fileRel, err := filepath.Rel(s.cfg.Root, path)
		if err != nil {
			return nil
		}
		fileRel = filepath.ToSlash(fileRel)
		switch {
		case isRequestFile(name):
			if len(out.Files) == maxListFiles {
				out.Truncated = true
				return errListFull
			}
			dialect := "hurl"
			if strings.EqualFold(filepath.Ext(name), ".sonde") {
				dialect = "sonde"
			}
			out.Files = append(out.Files, listedFile{Path: fileRel, Dialect: dialect})
		case name == "sonde.yaml":
			if len(out.Projects) == maxListProjects {
				out.Truncated = true
				return nil
			}
			out.Projects = append(out.Projects, loadProject(path, fileRel))
		}
		return nil
	})
	if err != nil && !errors.Is(err, errListFull) {
		return nil, out, err
	}
	s.audit("sonde_list", start, "dir=%q files=%d projects=%d", rel, len(out.Files), len(out.Projects))
	return nil, out, nil
}

// loadProject lists the environments of a sonde.yaml.
func loadProject(path, rel string) project {
	p := project{Path: rel, Environments: []string{}}
	proj, err := config.LoadProject(path)
	if err != nil {
		p.Error = strings.ReplaceAll(err.Error(), path, rel)
		return p
	}
	for name := range proj.Environments {
		p.Environments = append(p.Environments, name)
	}
	slices.Sort(p.Environments)
	p.DefaultEnv = proj.Defaults.Env
	return p
}
