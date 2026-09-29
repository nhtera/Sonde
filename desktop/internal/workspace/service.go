// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

// Service is the workspace bindings of every mode: exactly the methods the
// page may call. Opening a folder is the window app's (Desktop) or the
// server's --root.
type Service struct{ w *Workspace }

// NewService returns the bindings over w.
func NewService(w *Workspace) *Service { return &Service{w: w} }

// Project returns the open project, or nil.
func (s *Service) Project() *Project { return s.w.Project() }

// Tree returns the project tree.
func (s *Service) Tree() (*Node, error) { return s.w.Tree() }

// Index lists every request of every request file, for search.
func (s *Service) Index() ([]Request, error) { return s.w.Index() }

// Requests lists the requests of file.
func (s *Service) Requests(file string) ([]Request, error) { return s.w.Requests(file) }

// Read returns file's text and hash.
func (s *Service) Read(file string) (*FileText, error) { return s.w.Read(file) }

// Save writes text to file if its hash is still expectedHash ("" for a
// new file) and returns the new hash.
func (s *Service) Save(file, text, expectedHash string) (string, error) {
	return s.w.Save(file, text, expectedHash)
}

// NewFile creates an empty request file and returns its path.
func (s *Service) NewFile(dir, name string) (string, error) { return s.w.NewFile(dir, name) }

// NewRequest appends a request to file and returns the new text.
func (s *Service) NewRequest(file, method, url, expectedHash string) (*FileText, error) {
	return s.w.NewRequest(file, method, url, expectedHash)
}

// Duplicate copies file next to itself and returns the copy's path.
func (s *Service) Duplicate(file string) (string, error) { return s.w.Duplicate(file) }

// Rename renames file and returns its new path.
func (s *Service) Rename(file, newName string) (string, error) { return s.w.Rename(file, newName) }

// CopyPath returns file's absolute path, for the clipboard.
func (s *Service) CopyPath(file string) (string, error) { return s.w.CopyPath(file) }
