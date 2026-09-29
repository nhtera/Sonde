// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package copyas renders a request file as commands to copy: curl command
// lines (the engine's own rendering, with the project's variables, the
// overrides and the captures of the file's last run) and the `sonde`
// command that reproduces a run, a Send or a test run (the run's own
// invocation, quoted for a shell). Without reveal, credentials are
// redacted and the text goes to the page; with reveal (the window app
// only), the text goes to the clipboard from Go, concealed.
package copyas

import (
	"context"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/runflags"
	"github.com/nhtera/sonde/internal/runplan"
)

// Request selects what to copy.
type Request struct {
	File   string `json:"file"`
	Source string `json:"source"`
	Env    string `json:"env"`
	// Entry selects one entry (curl, and the Send of a sonde command);
	// 0 is every entry.
	Entry int `json:"entry"`
	// Kind is the sonde command's: "run", "send" or "test".
	Kind  string   `json:"kind"`
	Files []string `json:"files"` // a test run's files
	// Shell is "posix", "powershell" or "cmd".
	Shell string `json:"shell"`
}

// Text is text to copy with a note for the user.
type Text struct {
	Text string `json:"text"`
	Note string `json:"note,omitempty"`
}

// Planner gives a planned run's options and a run's invocation.
type Planner interface {
	Planned(ctx context.Context, file, source, env string, withCaptures bool) (engine.Options, error)
	Invocation(cmd, env, data, kind string, files []string) runplan.Invocation
}

// Copier renders commands.
type Copier struct {
	plan    Planner
	project func() string // the project folder, for the note
}

// New returns a copier.
func New(p Planner, projectDir func() string) *Copier { return &Copier{plan: p, project: projectDir} }

// Curl renders the curl command lines of req.File (or of entry
// req.Entry), one per line, with the captures of the file's last run.
func (c *Copier) Curl(ctx context.Context, req Request, reveal bool) (*Text, error) {
	opts, err := c.plan.Planned(ctx, req.File, req.Source, req.Env, true)
	if err != nil {
		return nil, err
	}
	opts.FromEntry, opts.ToEntry = req.Entry, req.Entry
	runner := engine.NewRunner(opts)
	if reveal {
		enginex.RevealCurl(runner)
	}
	entries, err := runner.RenderCurl(ctx, req.File, []byte(req.Source))
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	var lines, undefined, notes []string
	for _, e := range entries {
		switch {
		case e.Err != nil:
			return nil, apperr.New(apperr.Invalid, "request "+strconv.Itoa(e.Index)+": "+runner.Redact(e.Err.Error()))
		case e.Skipped != "":
			notes = append(notes, "Request "+strconv.Itoa(e.Index)+" has no curl command: "+e.Skipped+".")
		default:
			lines = append(lines, e.Command)
		}
		undefined = append(undefined, e.Undefined...)
	}
	if len(undefined) > 0 {
		notes = append(notes, "Not defined yet (run the file first): "+strings.Join(dedupe(undefined), ", ")+".")
	}
	return &Text{Text: strings.Join(lines, "\n"), Note: strings.Join(notes, " ")}, nil
}

// Sonde renders the `sonde` command that reproduces a run (kind "run"),
// a Send of entry req.Entry ("send": entries 1…N, since the CLI starts
// from the file, not from earlier captures) or a test run ("test"). It
// runs in the project folder.
func (c *Copier) Sonde(req Request, reveal bool) (*Text, error) {
	cmd, files, note := "run", []string{req.File}, ""
	switch req.Kind {
	case "", "run":
	case "send":
		note = "Runs requests 1–" + strconv.Itoa(req.Entry) + ": the CLI starts from the file, not from the app's earlier captures."
	case "test":
		cmd, files = "test", req.Files
	default:
		return nil, apperr.New(apperr.Invalid, "unknown command kind "+req.Kind)
	}
	inv := c.plan.Invocation(cmd, req.Env, "", req.Kind, files)
	if req.Kind == "send" {
		inv.ToEntry = req.Entry
		inv.Set["to-entry"] = true
	}
	dialect, err := dialectOf(req.Shell)
	if err != nil {
		return nil, err
	}
	text, err := runflags.Shell(&inv, dialect, reveal)
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	where := "Run it in " + c.project() + "."
	if note == "" {
		note = where
	} else {
		note += " " + where
	}
	return &Text{Text: text, Note: note}, nil
}

func dialectOf(shell string) (runflags.Dialect, error) {
	switch shell {
	case "", "posix":
		return runflags.POSIX, nil
	case "powershell":
		return runflags.PowerShell, nil
	case "cmd":
		return runflags.Cmd, nil
	}
	return 0, apperr.New(apperr.Invalid, "unknown shell "+shell)
}

func dedupe(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// Service is the Copy as bindings: redacted text for the page.
type Service struct{ c *Copier }

// NewService returns the bindings over c.
func NewService(c *Copier) *Service { return &Service{c: c} }

// Curl renders curl command lines, credentials redacted.
func (s *Service) Curl(ctx context.Context, req Request) (*Text, error) {
	return s.c.Curl(ctx, req, false)
}

// Sonde renders the sonde command, credentials as variable references.
func (s *Service) Sonde(req Request) (*Text, error) { return s.c.Sonde(req, false) }

// Reveal is the window app's Copy as with credentials: the text goes to
// the clipboard from Go (concealed, cleared after a minute), never to the
// page; the page gets only the note.
type Reveal struct {
	c     *Copier
	write func(text string) error
}

// NewReveal returns the reveal bindings; write puts a secret on the
// clipboard.
func NewReveal(c *Copier, write func(string) error) *Reveal { return &Reveal{c: c, write: write} }

// Curl copies curl command lines with their credentials.
func (r *Reveal) Curl(ctx context.Context, req Request) (string, error) {
	t, err := r.c.Curl(ctx, req, true)
	if err != nil {
		return "", err
	}
	return t.Note, r.write(t.Text)
}

// Sonde copies the sonde command with its credentials.
func (r *Reveal) Sonde(req Request) (string, error) {
	t, err := r.c.Sonde(req, true)
	if err != nil {
		return "", err
	}
	return t.Note, r.write(t.Text)
}
