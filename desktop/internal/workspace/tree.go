// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/nhtera/sonde/internal/sandbox"
)

// File kinds in the tree.
const (
	KindDir     = "dir"
	KindRequest = "request" // .hurl, .sonde
	KindConfig  = "config"  // sonde.yaml
	KindSecrets = "secrets" // *.secrets
	KindData    = "data"    // .csv, .json
	KindFile    = "file"
)

// Node is a folder or file of the project tree. Path is project-relative
// and slash-separated ("" for the root).
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Kind     string  `json:"kind"`
	Children []*Node `json:"children,omitempty"`
}

// maxTreeFiles bounds the tree of a huge folder opened by mistake.
const maxTreeFiles = 20000

// tree walks root: folders first, then files, each by name; dot folders,
// dot files and node_modules are skipped.
func tree(root *sandbox.Root) (*Node, error) {
	n := 0
	var walk func(rel string) (*Node, error)
	walk = func(rel string) (*Node, error) {
		node := &Node{Name: path.Base(rel), Path: rel, Kind: KindDir, Children: []*Node{}}
		if rel == "" {
			node.Name = ""
		}
		dir := rel
		if dir == "" {
			dir = "."
		}
		entries, err := root.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var dirs, files []*Node
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			child := path.Join(rel, name)
			if e.IsDir() {
				if ignoredDir(name) {
					continue
				}
				sub, err := walk(child)
				if err != nil {
					continue // an unreadable folder is left out
				}
				dirs = append(dirs, sub)
				continue
			}
			if t := e.Type(); !t.IsRegular() && t&fs.ModeSymlink == 0 {
				continue // sockets, devices, pipes
			}
			if n++; n > maxTreeFiles {
				break
			}
			files = append(files, &Node{Name: name, Path: child, Kind: kindOf(name)})
		}
		byName := func(a, b *Node) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }
		slices.SortFunc(dirs, byName)
		slices.SortFunc(files, byName)
		node.Children = append(append(node.Children, dirs...), files...)
		return node, nil
	}
	return walk("")
}

func kindOf(name string) string {
	switch ext := strings.ToLower(path.Ext(name)); {
	case ext == ".hurl" || ext == ".sonde":
		return KindRequest
	case name == "sonde.yaml" || name == "sonde.yml":
		return KindConfig
	case ext == ".secrets":
		return KindSecrets
	case ext == ".csv" || ext == ".json":
		return KindData
	}
	return KindFile
}
