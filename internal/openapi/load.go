// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package openapi validates responses against an OpenAPI 3.x description
// (the engine.ResponseValidator of --openapi) and generates request files
// from one (sonde import openapi).
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	yaml "go.yaml.in/yaml/v3"
)

// MaxSpecBytes caps a spec file and each file it references.
const MaxSpecBytes = 64 << 20

// fetchTimeout bounds each remote read.
const fetchTimeout = 30 * time.Second

// LoadOptions tell where a spec may be read from.
type LoadOptions struct {
	// AllowRemote allows an http(s) spec location and remote $ref
	// targets; without it both are errors.
	AllowRemote bool
	// HTTPClient fetches remote documents (nil: a client with a timeout).
	HTTPClient *http.Client
}

// Spec is a loaded OpenAPI description. It is read only once loaded and
// shared by every validator of a run.
type Spec struct {
	doc *openapi3.T
	// checkDoc is doc announced as OpenAPI 3.0, handed to the library's
	// response checks: they then validate schemas with the built-in
	// validator, never with the JSON Schema 2020 compiler, which reads
	// $schema and $dynamicRef targets from local files and recompiles
	// every schema on every response.
	checkDoc *openapi3.T
	// servers are the base URLs of the spec, variables replaced by their
	// defaults.
	servers []string
	// templates are the path templates, in the spec's order.
	templates []string
	router    *Router
}

// Load reads the spec at location: a file path, or an http(s) URL with
// AllowRemote. A $ref may name another file inside the directory of a
// local spec, or, with AllowRemote, a remote document; Swagger 2.0 is
// converted to OpenAPI 3.
func Load(ctx context.Context, location string, opt LoadOptions) (*Spec, error) {
	r := &reader{ctx: ctx, opt: opt}
	var base *url.URL
	if isRemote(location) {
		if !opt.AllowRemote {
			return nil, fmt.Errorf("%s: remote specs are disabled, use --openapi-allow-remote to fetch them", location)
		}
		u, err := url.Parse(location)
		if err != nil {
			return nil, err
		}
		base = u
	} else {
		abs, err := filepath.Abs(location)
		if err != nil {
			return nil, err
		}
		root, err := os.OpenRoot(filepath.Dir(abs))
		if err != nil {
			return nil, err
		}
		defer root.Close() //nolint:errcheck // read only
		r.root, r.rootDir = root, filepath.Dir(abs)
		base = &url.URL{Path: filepath.ToSlash(abs)}
	}
	data, err := r.read(base)
	if err != nil {
		return nil, err
	}
	doc, err := r.parse(data, base)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", location, err)
	}
	return newSpec(doc), nil
}

func isRemote(location string) bool {
	return strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")
}

// parse loads a document, converting Swagger 2.0. A malformed document
// can make the library panic: that is an error too.
func (r *reader) parse(data []byte, base *url.URL) (doc *openapi3.T, err error) {
	defer func() {
		if p := recover(); p != nil {
			doc, err = nil, fmt.Errorf("malformed spec: %v", p)
		}
	}()
	var head struct {
		Swagger string `yaml:"swagger"`
		OpenAPI string `yaml:"openapi"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("not a YAML or JSON document: %w", err)
	}
	loader := openapi3.NewLoader()
	loader.Context = r.ctx
	// Every external read goes through r.readURI, which applies the
	// remote and directory policies.
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, u *url.URL) ([]byte, error) { return r.read(u) }
	switch {
	case head.Swagger != "":
		if !strings.HasPrefix(head.Swagger, "2.") {
			return nil, fmt.Errorf("unsupported swagger version %q", head.Swagger)
		}
		var generic any
		if err := yaml.Unmarshal(data, &generic); err != nil {
			return nil, err
		}
		j, err := json.Marshal(generic)
		if err != nil {
			return nil, err
		}
		var doc2 openapi2.T
		if err := json.Unmarshal(j, &doc2); err != nil {
			return nil, err
		}
		doc, err := openapi2conv.ToV3(&doc2)
		if err != nil {
			return nil, err
		}
		if len(doc.Servers) == 0 && doc2.BasePath != "" {
			// Without a host, the conversion drops the base path.
			doc.AddServer(&openapi3.Server{URL: doc2.BasePath})
		}
		if err := loader.ResolveRefsIn(doc, base); err != nil {
			return nil, err
		}
		return doc, nil
	case strings.HasPrefix(head.OpenAPI, "3."):
		return loader.LoadFromDataWithPath(data, base)
	case head.OpenAPI != "":
		return nil, fmt.Errorf("unsupported openapi version %q", head.OpenAPI)
	}
	return nil, errors.New("not an OpenAPI document (no openapi or swagger key)")
}

// reader reads spec documents: files inside rootDir, and remote documents
// when allowed.
type reader struct {
	ctx     context.Context
	opt     LoadOptions
	root    *os.Root
	rootDir string
}

func (r *reader) read(u *url.URL) ([]byte, error) {
	switch u.Scheme {
	case "http", "https":
		if !r.opt.AllowRemote {
			return nil, fmt.Errorf("remote reference %s is disabled, use --openapi-allow-remote to fetch it", u.Redacted())
		}
		return r.fetch(u)
	case "", "file":
	default:
		return nil, fmt.Errorf("unsupported reference %s", u.Redacted())
	}
	if r.root == nil {
		return nil, fmt.Errorf("file reference %s in a remote spec", u.Path)
	}
	rel, err := filepath.Rel(r.rootDir, filepath.FromSlash(u.Path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s: a reference must stay inside the directory of the spec", u.Path)
	}
	// Checked before opening: opening a FIFO blocks.
	if fi, err := r.root.Stat(rel); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", u.Path)
	}
	f, err := r.root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read only
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", u.Path)
	}
	return readLimited(f, u.Path)
}

func (r *reader) fetch(u *url.URL) ([]byte, error) {
	client := r.opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, u.String(), nil) //nolint:gosec // G704: remote fetching is an explicit opt-in
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req) //nolint:gosec // G107, G704: remote fetching is an explicit opt-in
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // read only
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP status %d", u.Redacted(), resp.StatusCode)
	}
	return readLimited(resp.Body, u.Redacted())
}

func readLimited(rd io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(rd, MaxSpecBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSpecBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", name, MaxSpecBytes)
	}
	return bytes.Clone(data), nil
}

func newSpec(doc *openapi3.T) *Spec {
	check := *doc
	check.OpenAPI = "3.0.3"
	check.JSONSchemaDialect = ""
	s := &Spec{doc: doc, checkDoc: &check}
	for _, srv := range doc.Servers {
		if srv == nil {
			continue
		}
		u := srv.URL
		for name, v := range srv.Variables {
			if v != nil {
				u = strings.ReplaceAll(u, "{"+name+"}", v.Default)
			}
		}
		s.servers = append(s.servers, u)
	}
	if doc.Paths != nil {
		s.templates = doc.Paths.InMatchingOrder()
	}
	s.router = NewRouter(s.templates, s.servers)
	return s
}

// Servers returns the base URLs of the spec, variables replaced by their
// defaults.
func (s *Spec) Servers() []string { return s.servers }
