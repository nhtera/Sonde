// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package importsvc imports curl commands, Postman collections, Bruno
// (OpenCollection) collections, .http files and OpenAPI specs into the
// project, with the converters `sonde import` uses and the same options,
// so the files written are the command's, byte for byte. A preview shows
// them first; files that exist are overwritten only when named; a curl
// command's credentials can be lifted into the environment's secrets file.
package importsvc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/curl"
	"github.com/nhtera/sonde/internal/convert/httpfile"
	"github.com/nhtera/sonde/internal/convert/opencollection"
	"github.com/nhtera/sonde/internal/convert/postman"
	"github.com/nhtera/sonde/internal/openapi"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
)

// Kinds of import.
const (
	Curl           = "curl"
	Postman        = "postman"
	OpenCollection = "opencollection"
	HTTP           = "http"
	OpenAPI        = "openapi"
)

// Secrets is where lifted values go (envsvc).
type Secrets interface {
	// SetSecret writes secret name of env to its secrets file.
	SetSecret(env, name, secret string) error
	// Names lists the variables and secrets env has.
	Names(env string) map[string]bool
}

// Service imports into the open project.
type Service struct {
	project func() *sandbox.Root
	handles *handles.Table
	secrets Secrets
	stage   *sandbox.Root // the app's cache: uploaded and pasted inputs

	mu     sync.Mutex
	inputs map[string]staged
	pasted map[string]string // staged pasted texts: their hash → path
}

// New returns the import service; stage keeps the uploaded and pasted
// inputs (in an import folder, emptied by Reset).
func New(project func() *sandbox.Root, h *handles.Table, secrets Secrets, stage *sandbox.Root) *Service {
	s := &Service{project: project, handles: h, secrets: secrets, stage: stage}
	s.Reset() // what an earlier run left
	return s
}

// Request is an import: what, from where, with which options, into which
// project folder.
type Request struct {
	Kind string `json:"kind"`
	// Input is a staged input; without one, Text is the input (pasted).
	Input string `json:"input"`
	Text  string `json:"text"`
	// Environments are staged inputs: Postman environments, or the env
	// files of an .http file.
	Environments []string `json:"environments"`
	// Group is Postman's layout (request: a file per request, folder: a
	// file per folder) or OpenAPI's (tag, path, flat).
	Group      string `json:"group"`
	BaseURLVar string `json:"baseUrlVar"`
	Ext        string `json:"ext"` // hurl (default) or sonde
	// Folder is the project folder written into ("" the project's).
	Folder string `json:"folder"`
	// Env receives the lifted secrets; Lift picks them (Candidate IDs).
	// Without a pick (null), every candidate is lifted when there is an
	// Env: no value reaches the page unless the user keeps it in.
	Env  string `json:"env"`
	Lift []int  `json:"lift"`
	// Target is the open file a curl command is inserted into, TargetText
	// its text: its dialect and its captures' names count.
	Target     string `json:"target"`
	TargetText string `json:"targetText"`
}

// File is a file the import writes.
type File struct {
	Path   string `json:"path"` // project path
	Text   string `json:"text"`
	Exists bool   `json:"exists"` // it would replace a file
	Secret bool   `json:"secret"` // an empty secrets stub (0600)
}

// Counts sum up an import for its result.
type Counts struct {
	Files         int `json:"files"`
	Requests      int `json:"requests"`
	Environments  int `json:"environments"`
	StatusChecks  int `json:"statusChecks"`  // expected statuses written as HTTP lines
	PathVariables int `json:"pathVariables"` // :id segments written as {{id}}
	SecretStubs   int `json:"secretStubs"`
	Scripts       int `json:"scripts"` // scripts kept as comments
}

// Warning is something the import could not carry over as is.
type Warning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Preview is what an import would write.
type Preview struct {
	Files      []File      `json:"files"`
	Warnings   []Warning   `json:"warnings"`
	Skipped    []Warning   `json:"skipped"` // Kind: the item's name
	Counts     Counts      `json:"counts"`
	Project    string      `json:"project"` // "created", "kept" (sonde.yaml exists) or ""
	Candidates []Candidate `json:"candidates"`
}

// Written is what an import wrote.
type Written struct {
	Files   []string `json:"files"`   // project paths written
	Kept    []string `json:"kept"`    // existing files left as they were
	Secrets []string `json:"secrets"` // lifted, by name
	Counts  Counts   `json:"counts"`
}

// plan is an import computed: the converter's output and the files.
type plan struct {
	root     *sandbox.Root
	dir      string // absolute folder written into
	rel      string // its project path ("" the project folder)
	out      convert.Output
	res      *convert.Result
	files    []convert.PlannedFile
	exists   map[string]bool
	cands    []candidate
	lifted   map[string]string
	password bool     // a password was lifted
	unlifted []string // credentials that can not be lifted (escaped)
	data     []byte   // the main input, for counts and suggestions
	postOpts postman.Options
}

// Preview computes an import without writing anything.
func (s *Service) Preview(ctx context.Context, req Request) (*Preview, error) {
	p, err := s.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	pv := &Preview{Files: []File{}, Warnings: []Warning{}, Skipped: []Warning{}, Counts: p.counts(), Candidates: []Candidate{}}
	for _, f := range p.files {
		pv.Files = append(pv.Files, File{Path: p.project(f.Path), Text: string(f.Data), Exists: p.exists[f.Path], Secret: f.Perm == 0o600})
	}
	for _, w := range p.out.Warnings {
		// "a password in plain text": not once the password is lifted.
		if w.Kind == convert.WarnSecret && p.password {
			continue
		}
		pv.Warnings = append(pv.Warnings, Warning{Kind: w.Kind, Message: w.Message})
	}
	for _, where := range p.unlifted {
		pv.Warnings = append(pv.Warnings, Warning{Kind: convert.WarnSecret, Message: where + ": a credential written with escapes stays in the file; move it to a secret by hand"})
	}
	for _, sk := range p.out.Skipped {
		pv.Skipped = append(pv.Skipped, Warning{Kind: sk.Name, Message: sk.Reason})
	}
	switch envs := p.environments(); {
	case p.res.Project != "":
		pv.Project = "created"
		if p.rel != "" && len(envs) > 0 {
			pv.Warnings = append(pv.Warnings, Warning{Kind: "environments", Message: fmt.Sprintf("The environments (%s) go to %s/sonde.yaml: the app uses the project's own sonde.yaml, so add them there to run with them here", strings.Join(envs, ", "), p.rel)})
		}
	case p.res.ProjectSkipped:
		pv.Project = "kept"
		if len(envs) > 0 {
			pv.Warnings = append(pv.Warnings, Warning{Kind: "environments", Message: fmt.Sprintf("sonde.yaml is there already and is never replaced: the environments %s are not added to it", strings.Join(envs, ", "))})
		}
	}
	for _, c := range p.cands {
		pv.Candidates = append(pv.Candidates, c.Candidate)
	}
	return pv, nil
}

// Write imports: the lifted secrets first (a request never names a secret
// that is not there), then every file, except the existing ones not
// named in overwrite.
func (s *Service) Write(ctx context.Context, req Request, overwrite []string) (*Written, error) {
	p, err := s.plan(ctx, req)
	if err != nil {
		return nil, err
	}
	w := &Written{Files: []string{}, Kept: []string{}, Secrets: []string{}, Counts: p.counts()}
	keep := func(f convert.PlannedFile) bool {
		return p.exists[f.Path] && !slices.Contains(overwrite, p.project(f.Path))
	}
	// The lifted values go only with the file that names them (a curl
	// import has one).
	if len(p.lifted) > 0 && len(p.files) > 0 && !keep(p.files[0]) {
		if err := s.writeSecrets(req.Env, p.lifted); err != nil {
			return nil, err
		}
		for name := range p.lifted {
			w.Secrets = append(w.Secrets, name)
		}
		slices.Sort(w.Secrets)
	}
	for _, f := range p.files {
		rel := p.project(f.Path)
		if keep(f) {
			w.Kept = append(w.Kept, rel)
			continue
		}
		// In the project's Root: folders created, nothing written outside.
		if err := p.root.WriteFileAtomic(rel, f.Data, f.Perm); err != nil {
			done := append(w.Secrets, w.Files...)
			if len(done) == 0 {
				return nil, apperr.Wrap(apperr.Denied, err)
			}
			return nil, apperr.New(apperr.Denied, fmt.Sprintf("%v (already written: %s)", err, strings.Join(done, ", ")))
		}
		w.Files = append(w.Files, rel)
	}
	return w, nil
}

func (s *Service) writeSecrets(env string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if env == "" {
		return apperr.New(apperr.Invalid, "pick the environment the secrets go to")
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		if err := s.secrets.SetSecret(env, n, values[n]); err != nil {
			return err
		}
	}
	return nil
}

// CurlText converts a pasted curl command for the open file: its
// requests, credentials lifted (written to req.Env's secrets file).
func (s *Service) CurlText(req Request) (string, error) {
	p, err := s.curlPlan(req)
	if err != nil {
		return "", err
	}
	return string(syntax.Format(p.out.Files[0].File)), nil
}

// SaveSecrets writes the values CurlText lifted, once its requests are in
// the file (never before: an edit refused would leave them unused).
func (s *Service) SaveSecrets(req Request) ([]string, error) {
	p, err := s.curlPlan(req)
	if err != nil {
		return nil, err
	}
	if err := s.writeSecrets(req.Env, p.lifted); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(p.lifted))
	for n := range p.lifted {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}

// curlPlan converts a pasted curl command for req.Target, in its dialect.
func (s *Service) curlPlan(req Request) (*plan, error) {
	req.Kind, req.Folder = Curl, ""
	if strings.HasSuffix(req.Target, ".sonde") {
		req.Ext = "sonde"
	} else {
		req.Ext = "hurl"
	}
	return s.plan(context.Background(), req)
}

// plan runs the converter and plans the files as the command would.
func (s *Service) plan(ctx context.Context, req Request) (*plan, error) {
	root := s.project()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(req.Folder)))
	if rel == "." || rel == "/" {
		rel = ""
	}
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") && part != "" {
			return nil, apperr.New(apperr.Denied, "import into a folder of the project, not "+req.Folder)
		}
	}
	dir := root.Dir()
	if rel != "" {
		// The converters read the folder by path (what exists there, the
		// spec's path from it); every write goes through the Root.
		abs, err := root.Path(rel) //nolint:forbidigo // read only, see above
		if err != nil {
			return nil, apperr.New(apperr.Denied, "import into a folder of the project, not "+req.Folder)
		}
		// As a project path (an absolute one given), checked again once
		// links are followed: no dot folder (.git) through a link.
		if rel, err = projectRel(root.Dir(), abs); err != nil {
			return nil, apperr.New(apperr.Denied, "import into a folder of the project, not "+req.Folder)
		}
		dir = abs
	}
	ext := req.Ext
	if ext == "" {
		ext = "hurl"
	}
	opts := convert.Options{Output: dir, Ext: ext, DryRun: true}
	p := &plan{root: root, dir: dir, rel: rel, exists: map[string]bool{}}
	if err := s.convert(ctx, req, opts, p); err != nil {
		return nil, err
	}
	if req.Kind == Curl && len(p.out.Files) == 1 {
		if err := s.liftCurl(req, opts.Dialect(), p); err != nil {
			return nil, err
		}
	}
	_, _, err := convert.Plan(dir, p.out, opts)
	var conflicts *convert.ErrConflicts
	switch {
	case errors.As(err, &conflicts):
		for _, f := range conflicts.Files {
			p.exists[f] = true
		}
	case err != nil:
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	opts.Force = true
	if p.res, p.files, err = convert.Plan(dir, p.out, opts); err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	return p, nil
}

// convert runs req's converter with the command's options.
func (s *Service) convert(ctx context.Context, req Request, opts convert.Options, p *plan) error {
	d := opts.Dialect()
	name, path, data, err := s.source(req)
	if err != nil {
		return err
	}
	envPaths := make([]string, 0, len(req.Environments))
	for _, id := range req.Environments {
		st, err := s.input(id)
		if err != nil {
			return err
		}
		envPaths = append(envPaths, st.path)
	}
	p.data = data
	switch req.Kind {
	case Curl:
		p.out, err = curl.ImportInput(name, data, d)
	case Postman:
		envs, rerr := postman.ReadEnvironments(envPaths)
		if rerr != nil {
			return apperr.Wrap(apperr.Invalid, rerr)
		}
		p.postOpts = postman.Options{Group: or(req.Group, postman.GroupRequest), Environments: envs, Dialect: d}
		p.out, err = postman.Import(data, p.postOpts)
	case OpenCollection:
		p.out, err = opencollection.ImportPath(path, d)
	case HTTP:
		p.out, err = httpfile.Import(httpfile.StemFor(name), data, d, httpfile.Options{EnvFiles: envPaths})
	case OpenAPI:
		p.out, err = openapi.Import(ctx, path, openapi.ImportOptions{Group: or(req.Group, "tag"), BaseURLVar: or(req.BaseURLVar, "base_url"), Dialect: d, Output: opts.Output})
	default:
		return apperr.New(apperr.Invalid, "unknown import "+req.Kind)
	}
	if err != nil {
		return apperr.Wrap(apperr.Invalid, err)
	}
	return nil
}

// source reads req's main input: its name as the command would be given
// it, its path on disk and its content (nil for a folder).
func (s *Service) source(req Request) (name, path string, data []byte, err error) {
	if req.Input == "" {
		if len(req.Text) > convert.MaxInput {
			return "", "", nil, apperr.New(apperr.Invalid, "the text is larger than 64 MiB")
		}
		if strings.TrimSpace(req.Text) == "" {
			return "", "", nil, apperr.New(apperr.Invalid, "paste something to import")
		}
		data = []byte(req.Text)
		if req.Kind == OpenCollection || req.Kind == OpenAPI {
			// These read a path: the text is staged as a file.
			base := map[string]string{OpenCollection: "collection.yml", OpenAPI: "openapi.yaml"}[req.Kind]
			if path, err = s.stagePasted(base, data); err != nil {
				return "", "", nil, err
			}
			return path, path, data, nil
		}
		return "-", "", data, nil
	}
	st, err := s.input(req.Input)
	if err != nil {
		return "", "", nil, err
	}
	if st.dir {
		if req.Kind != OpenCollection {
			return "", "", nil, apperr.New(apperr.Invalid, "pick a file, not a folder")
		}
		return st.name, st.path, nil, nil
	}
	if data, err = convert.ReadInput(st.path, convert.MaxInput); err != nil {
		return "", "", nil, apperr.Wrap(apperr.Invalid, err)
	}
	return st.name, st.path, data, nil
}

// liftCurl offers the credentials of the converted command and lifts
// those req picks.
func (s *Service) liftCurl(req Request, d syntax.Dialect, p *plan) error {
	src := syntax.Format(p.out.Files[0].File)
	taken := map[string]bool{}
	if req.Env != "" && s.secrets != nil {
		taken = s.secrets.Names(req.Env)
	}
	// Names the target file captures would override a lifted one.
	for _, n := range captureNames(req.Target, req.TargetText) {
		taken[n] = true
	}
	cands, unlifted, err := candidates(src, d, taken)
	if err != nil {
		return apperr.Wrap(apperr.Invalid, err)
	}
	p.cands, p.unlifted = cands, unlifted
	// Without an environment the values have nowhere to go: none lifted.
	var picked []int
	if req.Env != "" {
		picked = req.Lift
		if picked == nil {
			for _, c := range cands {
				picked = append(picked, c.ID)
			}
		}
	}
	for _, c := range cands {
		if strings.HasPrefix(c.Where, "password of ") && slices.Contains(picked, c.ID) {
			p.password = true
		}
	}
	out, values, err := lift(src, d, cands, picked)
	if err != nil {
		return apperr.Wrap(apperr.Invalid, err)
	}
	if values == nil {
		return nil
	}
	f, err := syntax.Parse("curl", out, d)
	if err != nil {
		return apperr.Wrap(apperr.Invalid, err)
	}
	p.out.Files[0].File = f
	p.lifted = values
	return nil
}

// projectRel returns abs (in the project dir) as a project path, refusing
// one that is, or goes through a link to, a dot folder.
func projectRel(dir, abs string) (string, error) {
	for _, path := range []string{abs, resolved(abs)} {
		base := dir
		if path != abs {
			base = resolved(dir)
		}
		rel, err := filepath.Rel(base, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", errors.New("outside the project")
		}
		for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
			if strings.HasPrefix(part, ".") && part != "." {
				return "", errors.New("a dot folder")
			}
		}
	}
	rel, _ := filepath.Rel(dir, abs)
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

// resolved is path with its links followed, as far as it exists.
func resolved(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		if filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

// captureNames lists the capture names of a request file's text.
func captureNames(name, text string) []string {
	if text == "" {
		return nil
	}
	f, err := syntax.Parse(name, []byte(text), syntax.DialectFor(name))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range f.Entries {
		if e.Response == nil {
			continue
		}
		for _, sec := range e.Response.Sections {
			for _, c := range sec.Captures {
				if n, ok := literal(c.Name); ok {
					out = append(out, n)
				}
			}
		}
	}
	return out
}

// project returns the project path of rel, a path in the import folder.
func (p *plan) project(rel string) string {
	if p.rel == "" {
		return rel
	}
	return p.rel + "/" + rel
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
