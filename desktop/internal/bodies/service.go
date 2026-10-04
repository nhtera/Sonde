// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package bodies

import (
	"context"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/osfile"
)

// Route serves the store's bodies; a service with only ServeHTTP binds
// nothing, so the page reaches bodies by URL only.
type Route struct{ s *Store }

// NewRoute returns the body route over s.
func NewRoute(s *Store) *Route { return &Route{s: s} }

func (r *Route) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.s.ServeHTTP(w, req) }

// Desktop is the window app's body bindings: saving a response (its raw
// bytes, to a file the user picks) and opening one in another app.
type Desktop struct {
	s *Store
	// pickSave shows the save dialog; "" when canceled.
	pickSave func(suggested string) (string, error)

	mu      sync.Mutex
	tempDir string
}

// NewDesktop returns the window-only body bindings.
func NewDesktop(s *Store, pickSave func(suggested string) (string, error)) *Desktop {
	return &Desktop{s: s, pickSave: pickSave}
}

// SaveResponse writes response body id, as received (decoded, not
// redacted: the user's own data), to a file the user picks; it returns
// the file's name, "" when canceled.
func (d *Desktop) SaveResponse(id string) (string, error) {
	data, ct, ok := d.s.Raw(id)
	if !ok {
		return "", apperr.New(apperr.NotFound, "this response is no longer in memory; run the request again")
	}
	path, err := d.pickSave("response" + extension(ct))
	if err != nil || path == "" {
		return "", err
	}
	//nolint:forbidigo,gosec // a file the user picked in the save dialog
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return filepath.Base(path), nil
}

// allowed are the extensions a body opens with in another app; any other
// type opens as text.
var allowed = map[string]bool{".pdf": true, ".png": true, ".jpg": true, ".gif": true, ".txt": true, ".json": true, ".xml": true}

// OpenExternally opens body id (redacted) in the system's app for its
// type. The file is private (0700 folder), has an allowlisted extension
// (else .txt) and is marked as downloaded (quarantine, Mark of the Web).
func (d *Desktop) OpenExternally(_ context.Context, id string) error {
	data, ct, ok := d.s.Redacted(id)
	if !ok {
		return apperr.New(apperr.NotFound, "this response is no longer available")
	}
	d.mu.Lock()
	if d.tempDir == "" {
		dir, err := os.MkdirTemp("", "sonde-open-")
		if err != nil {
			d.mu.Unlock()
			return err
		}
		d.tempDir = dir
	}
	dir := d.tempDir
	d.mu.Unlock()
	ext := extension(ct)
	if !allowed[ext] {
		ext = ".txt"
	}
	path := filepath.Join(dir, "response-"+id[:8]+ext)
	//nolint:forbidigo // a private temp file for another app
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	markDownloaded(path)
	return osfile.Open(path)
}

// ServiceShutdown removes the files opened externally when the app quits.
func (d *Desktop) ServiceShutdown() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tempDir != "" {
		_ = os.RemoveAll(d.tempDir) //nolint:forbidigo // our own temp folder
		d.tempDir = ""
	}
	return nil
}

// extension picks a file extension for a content type.
func extension(ct string) string {
	mt, _, _ := mime.ParseMediaType(ct)
	switch {
	case mt == "application/json" || strings.HasSuffix(mt, "+json"):
		return ".json"
	case mt == "application/xml" || mt == "text/xml" || strings.HasSuffix(mt, "+xml"):
		return ".xml"
	case mt == "image/jpeg":
		return ".jpg"
	case mt == "image/png":
		return ".png"
	case mt == "image/gif":
		return ".gif"
	case mt == "application/pdf":
		return ".pdf"
	case mt == "text/html":
		return ".html"
	case strings.HasPrefix(mt, "text/"):
		return ".txt"
	}
	return ".bin"
}
