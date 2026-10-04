// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/example"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/osfile"
	"github.com/nhtera/sonde/internal/sandbox"
)

// recentFile keeps the recent projects in the app's config folder.
const recentFile = "recent.json"

// maxRecent is how many recent projects are kept.
const maxRecent = 10

// Recent is a recently opened project.
type Recent struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Dir      string    `json:"dir"`
	OpenedAt time.Time `json:"openedAt"`
}

// Desktop is the workspace bindings of the window app only: opening
// folders, the recent list, and the file manager and trash. Server mode
// serves the one project given with --root and has none of these.
type Desktop struct {
	ws      *Workspace
	config  *sandbox.Root
	handles *handles.Table
	// pickFolder shows the native folder dialog; "" when canceled.
	pickFolder func() (string, error)
}

// NewDesktop returns the window-only bindings over ws. config is the
// app's config folder; pickFolder shows the native folder dialog.
func NewDesktop(ws *Workspace, config *sandbox.Root, h *handles.Table, pickFolder func() (string, error)) *Desktop {
	return &Desktop{ws: ws, config: config, handles: h, pickFolder: pickFolder}
}

// OpenFolder asks for a folder and opens it; nil when canceled.
func (d *Desktop) OpenFolder() (*Project, error) {
	dir, err := d.pickFolder()
	if err != nil || dir == "" {
		return nil, err
	}
	return d.open(dir)
}

// OpenExample writes the example project into Documents/Sonde (Sonde in
// the home folder when Documents is missing or denied) and opens it. One there
// already is opened as it is.
func (d *Desktop) OpenExample() (*Project, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	// Documents, unless there is none or macOS denies access to it.
	dir, err := writeExample(filepath.Join(home, "Documents"))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		dir, err = writeExample(home)
	}
	if err != nil {
		return nil, fmt.Errorf("the example project could not be written: %w", err)
	}
	return d.open(dir)
}

// writeExample writes the example into base/Sonde, base an existing
// folder, through a file root on base; it returns the example's path.
func writeExample(base string) (string, error) {
	root, err := sandbox.Open(base)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	if fi, err := root.Stat("."); err != nil || !fi.IsDir() {
		return "", fs.ErrNotExist
	}
	rel, err := example.Write(root, "Sonde")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(rel)), nil
}

// OpenRecent opens the recent project id.
func (d *Desktop) OpenRecent(id string) (*Project, error) {
	for _, r := range d.Recent() {
		if r.ID == id {
			return d.open(r.Dir)
		}
	}
	return nil, apperr.New(apperr.NotFound, "that project is no longer in the recent list")
}

func (d *Desktop) open(dir string) (*Project, error) {
	p, err := d.ws.Open(dir)
	if err != nil {
		return nil, err
	}
	d.remember(p)
	return p, nil
}

// Recent lists the recent projects, newest first.
func (d *Desktop) Recent() []Recent {
	out := []Recent{}
	if data, err := d.config.ReadFile(recentFile); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func (d *Desktop) remember(p *Project) {
	sum := sha256.Sum256([]byte(p.Dir))
	id := hex.EncodeToString(sum[:8])
	list := slices.DeleteFunc(d.Recent(), func(r Recent) bool { return r.ID == id })
	list = append([]Recent{{ID: id, Name: p.Name, Dir: p.Dir, OpenedAt: time.Now().UTC()}}, list...)
	if len(list) > maxRecent {
		list = list[:maxRecent]
	}
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = d.config.WriteFileAtomic(recentFile, data, 0o600)
	}
}

// Reveal shows file in the system file manager.
func (d *Desktop) Reveal(_ context.Context, file string) error {
	p, err := d.ws.abs(file)
	if err != nil {
		return err
	}
	return osfile.Reveal(p)
}

// Trash moves file (or folder) to the system trash.
func (d *Desktop) Trash(ctx context.Context, file string) error {
	p, err := d.ws.abs(file)
	if err != nil {
		return err
	}
	return osfile.Trash(ctx, p)
}

// Picked is a file picked in a dialog, as the page may know it: its
// project path when it is inside the project; else only its name, and a
// new handle for CopyIntoProject (runs read files inside the project
// only).
type Picked struct {
	Path   string `json:"path,omitempty"`
	Name   string `json:"name"`
	Handle string `json:"handle,omitempty"`
}

// PickedFile tells where the file picked in a dialog (handle) is.
func (d *Desktop) PickedFile(handle string) (*Picked, error) {
	root := d.ws.Root()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	src, err := d.handles.Take(handle, handles.OpenFile)
	if err != nil {
		return nil, apperr.Wrap(apperr.Expired, err)
	}
	if fi, err := os.Stat(src); err != nil || !fi.Mode().IsRegular() {
		return nil, apperr.New(apperr.Invalid, "not a file: "+filepath.Base(src))
	}
	name := filepath.Base(src)
	resolved, err1 := filepath.EvalSymlinks(src)
	dir, err2 := filepath.EvalSymlinks(root.Dir())
	if err1 == nil && err2 == nil {
		if rel, err := filepath.Rel(dir, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			// A secret or dot file is never sent as a body.
			if d.ws.protected(rel) {
				return nil, apperr.New(apperr.Denied, name+" is protected: it is not sent")
			}
			return &Picked{Path: filepath.ToSlash(rel), Name: name}, nil
		}
	}
	h, err := d.handles.Put(src, handles.OpenFile)
	if err != nil {
		return nil, err
	}
	return &Picked{Name: name, Handle: h}, nil
}

// CopyIntoProject copies the file picked in a dialog (handle) into folder
// dir of the project and returns its path.
func (d *Desktop) CopyIntoProject(handle, dir string) (string, error) {
	src, err := d.handles.Take(handle, handles.OpenFile)
	if err != nil {
		return "", apperr.Wrap(apperr.Expired, err)
	}
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", apperr.New(apperr.Invalid, "not a file: "+filepath.Base(src))
	}
	data, err := os.ReadFile(src) //nolint:gosec // G304: a file the user picked in the native dialog
	if err != nil {
		return "", err
	}
	rel := path.Join(dir, filepath.Base(src))
	if _, err := d.ws.Save(rel, string(data), ""); err != nil {
		var e *apperr.Error
		if errors.As(err, &e) && e.Code == apperr.Conflict {
			return "", apperr.New(apperr.Conflict, rel+" already exists")
		}
		return "", err
	}
	return rel, nil
}
