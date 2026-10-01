// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import "context"

// API is the import service the page calls.
type API struct{ s *Service }

// NewAPI returns the page's import service over s.
func NewAPI(s *Service) *API { return &API{s: s} }

// Stage takes a file or folder picked in a dialog (kind "file" or "dir").
func (a *API) Stage(handle, kind string) (*Input, error) { return a.s.Stage(handle, kind) }

// Upload takes a file sent by the page (base64).
func (a *API) Upload(name, data string) (*Input, error) { return a.s.Upload(name, data) }

// Preview computes an import without writing.
func (a *API) Preview(ctx context.Context, req Request) (*Preview, error) {
	return a.s.Preview(ctx, req)
}

// Write imports, overwriting only the existing files named.
func (a *API) Write(ctx context.Context, req Request, overwrite []string) (*Written, error) {
	return a.s.Write(ctx, req, overwrite)
}

// CurlText converts a pasted curl command for the open file.
func (a *API) CurlText(req Request) (string, error) { return a.s.CurlText(req) }

// SaveSecrets writes the values a curl command inserted lifted.
func (a *API) SaveSecrets(req Request) ([]string, error) { return a.s.SaveSecrets(req) }

// Suggestions lists what a Postman import's scripts translate to.
func (a *API) Suggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	return a.s.Suggestions(ctx, req)
}

// Accept applies the suggestions of one imported file: the changes
// picked, by index (all of them when picked is null).
func (a *API) Accept(ctx context.Context, req Request, file string, picked []int) error {
	return a.s.Accept(ctx, req, file, picked)
}
