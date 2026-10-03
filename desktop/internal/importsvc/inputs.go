// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Input is a file or folder picked for an import: staged once (a dialog
// handle is good for one use) and named by its ID in every request.
type Input struct {
	ID   string `json:"id"`
	Name string `json:"name"` // its base name
	Dir  bool   `json:"dir"`
}

// staged is an input on disk. name is what the command would be given
// for it: its path, or "-" for pasted text (stdin), so that output stems
// match.
type staged struct {
	path, name string
	dir        bool
}

// Stage takes a dialog handle (kind "file" or "dir") as an input.
func (s *Service) Stage(handle, kind string) (*Input, error) {
	k := handles.OpenFile
	if kind == "dir" {
		k = handles.OpenDir
	}
	path, err := s.handles.Take(handle, k)
	if err != nil {
		return nil, apperr.New(apperr.Expired, "the file picked is no longer available: pick it again")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.NotFound, err)
	}
	if info.IsDir() != (kind == "dir") || !info.IsDir() && !info.Mode().IsRegular() {
		return nil, apperr.New(apperr.Invalid, "not a "+map[bool]string{true: "folder", false: "file"}[kind == "dir"]+": "+filepath.Base(path))
	}
	return s.add(staged{path: path, name: path, dir: info.IsDir()})
}

// Upload stages a file sent by the page (server mode), data in base64,
// under its base name (output stems follow it, as for that file).
func (s *Service) Upload(name, data string) (*Input, error) {
	if base64.StdEncoding.DecodedLen(len(data)) > convert.MaxInput {
		return nil, apperr.New(apperr.Invalid, "the file is larger than 64 MiB")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, apperr.New(apperr.Invalid, "not base64: "+err.Error())
	}
	// The page's file name, cut at either separator on every OS.
	base := name[strings.LastIndexAny(name, `/\`)+1:]
	if base == "" || base == "." || base == ".." {
		base = "upload"
	}
	path, err := s.temp(base, b)
	if err != nil {
		return nil, err
	}
	return s.add(staged{path: path, name: path})
}

func (s *Service) add(st staged) (*Input, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b[:])
	s.mu.Lock()
	s.inputs[id] = st
	s.mu.Unlock()
	return &Input{ID: id, Name: filepath.Base(st.path), Dir: st.dir}, nil
}

func (s *Service) input(id string) (staged, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.inputs[id]
	if !ok {
		return staged{}, apperr.New(apperr.Expired, "pick the file again")
	}
	return st, nil
}

// stageDir is the folder of the staged inputs in the stage Root.
const stageDir = "import"

// temp writes data as a new file named base in the staged inputs and
// returns its path.
func (s *Service) temp(base string, data []byte) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	name := stageDir + "/" + hex.EncodeToString(b[:]) + "/" + base
	if err := s.stage.WriteFileAtomic(name, data, 0o600); err != nil {
		return "", apperr.Wrap(apperr.Invalid, err)
	}
	// Our own name under the Root: the converters read it by path.
	return filepath.Join(s.stage.Dir(), filepath.FromSlash(name)), nil
}

// stagePasted stages pasted text read by path, once per text (a preview
// runs at every pause in typing).
func (s *Service) stagePasted(base string, data []byte) (string, error) {
	sum := sha256.Sum256(append([]byte(base+"\x00"), data...))
	key := hex.EncodeToString(sum[:])
	s.mu.Lock()
	path, ok := s.pasted[key]
	s.mu.Unlock()
	if ok {
		return path, nil
	}
	path, err := s.temp(base, data)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.pasted[key] = path
	s.mu.Unlock()
	return path, nil
}

// Reset forgets the staged inputs and deletes the uploaded and pasted
// ones (at start, and when another project opens).
func (s *Service) Reset() {
	s.mu.Lock()
	s.inputs = map[string]staged{}
	s.pasted = map[string]string{}
	s.layouts = nil
	s.mu.Unlock()
	removeTree(s.stage, stageDir)
}

// removeTree removes name and everything in it from root.
func removeTree(root *sandbox.Root, name string) {
	entries, _ := root.ReadDir(name)
	for _, e := range entries {
		child := name + "/" + e.Name()
		if e.IsDir() {
			removeTree(root, child)
			continue
		}
		_ = root.Remove(child)
	}
	_ = root.Remove(name)
}
