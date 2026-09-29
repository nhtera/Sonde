// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package jar keeps a request file's cookies between full runs when "Keep
// cookies" is on: the jar is a Netscape cookie file (what `sonde run -b
// FILE -c FILE` reads and writes) in the app's config folder, unredacted,
// 0600, written atomically. The page lists jars and cookies without their
// values.
package jar

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/cookiejar"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Dir is the jars' folder in the app's config folder.
const Dir = "cookies"

// Jars are the kept jars of the open project.
type Jars struct {
	config  *sandbox.Root
	project func() *sandbox.Root
	keep    func() bool

	mu sync.Mutex // one jar write at a time
}

// New returns the jars kept in config for the project project returns;
// keep reports whether "Keep cookies" is on.
func New(config *sandbox.Root, project func() *sandbox.Root, keep func() bool) *Jars {
	return &Jars{config: config, project: project, keep: keep}
}

func id(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// name is the jar file of project file file, relative to config.
func (j *Jars) name(file string) (string, bool) {
	root := j.project()
	if root == nil {
		return "", false
	}
	return path.Join(Dir, id(root.Dir()), id(file)+".txt"), true
}

// KeptJar returns the path of file's jar, for a run's -b, when keep
// cookies is on and the jar exists.
func (j *Jars) KeptJar(file string) string {
	if !j.keep() {
		return ""
	}
	name, ok := j.name(file)
	if !ok {
		return ""
	}
	if _, err := j.config.Stat(name); err != nil {
		return ""
	}
	return filepath.Join(j.config.Dir(), filepath.FromSlash(name))
}

// Keep writes file's jar after a full run, when keep cookies is on.
func (j *Jars) Keep(file string, cookies []engine.Cookie) {
	if !j.keep() {
		return
	}
	name, ok := j.name(file)
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_ = j.config.MkdirAll(path.Dir(name), 0o700)
	_ = cookiejar.Write(j.config, name, file, cookies, func(s string) string { return s })
}

// Jar is a kept jar, for the page.
type Jar struct {
	File    string   `json:"file"`
	Path    string   `json:"path"`
	Cookies []Cookie `json:"cookies"`
}

// Cookie is a kept cookie without its value.
type Cookie struct {
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Expires  int64  `json:"expires"` // Unix seconds; 0: a session cookie
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
}

var forFile = regexp.MustCompile(`(?m)^# Cookies for file <(.*)>$`)

// List returns the open project's jars, by file.
func (j *Jars) List() ([]Jar, error) {
	root := j.project()
	if root == nil {
		return nil, apperr.New(apperr.NotFound, "no project is open")
	}
	dir := path.Join(Dir, id(root.Dir()))
	entries, err := j.config.ReadDir(dir)
	if err != nil {
		return []Jar{}, nil //nolint:nilerr // no jars yet
	}
	out := []Jar{}
	for _, e := range entries {
		data, err := j.config.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		m := forFile.FindSubmatch(data)
		if m == nil {
			continue
		}
		jr := Jar{File: string(m[1]), Path: filepath.Join(j.config.Dir(), dir, e.Name()), Cookies: []Cookie{}}
		for _, c := range parse(data) {
			jr.Cookies = append(jr.Cookies, Cookie{Domain: c.Domain, Path: c.Path, Name: c.Name, Expires: c.Expires, Secure: c.HTTPS, HTTPOnly: c.HTTPOnly})
		}
		out = append(out, jr)
	}
	slices.SortFunc(out, func(a, b Jar) int { return strings.Compare(a.File, b.File) })
	return out, nil
}

// Delete removes one cookie from file's jar.
func (j *Jars) Delete(file, domain, cookiePath, name string) error {
	return j.rewrite(file, func(c engine.Cookie) bool {
		return c.Domain == domain && c.Path == cookiePath && c.Name == name
	})
}

// Clear empties file's jar; with file "", every jar of the project.
func (j *Jars) Clear(file string) error {
	if file != "" {
		n, ok := j.name(file)
		if !ok {
			return apperr.New(apperr.NotFound, "no project is open")
		}
		j.mu.Lock()
		defer j.mu.Unlock()
		_ = j.config.Remove(n)
		return nil
	}
	jars, err := j.List()
	if err != nil {
		return err
	}
	for _, jr := range jars {
		if err := j.Clear(jr.File); err != nil {
			return err
		}
	}
	return nil
}

func (j *Jars) rewrite(file string, drop func(engine.Cookie) bool) error {
	name, ok := j.name(file)
	if !ok {
		return apperr.New(apperr.NotFound, "no project is open")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	data, err := j.config.ReadFile(name)
	if err != nil {
		return apperr.New(apperr.NotFound, "no kept cookies for "+file)
	}
	kept := slices.DeleteFunc(parse(data), drop)
	return cookiejar.Write(j.config, name, file, kept, func(s string) string { return s })
}

// parse reads a Netscape cookie file, as the runner's cookie store does.
func parse(data []byte) []engine.Cookie {
	var out []engine.Cookie
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		c := engine.Cookie{Domain: f[0], IncludeSubdomain: f[1] == "TRUE", Path: f[2], HTTPS: f[3] == "TRUE", Name: f[5]}
		if d, ok := strings.CutPrefix(c.Domain, "#HttpOnly_"); ok {
			c.Domain, c.HTTPOnly = d, true
		}
		exp, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			continue
		}
		c.Expires = exp
		if len(f) > 6 {
			c.Value = strings.Join(f[6:], "\t")
		}
		out = append(out, c)
	}
	return out
}

// Service is the jar bindings.
type Service struct{ j *Jars }

// NewService returns the bindings over j.
func NewService(j *Jars) *Service { return &Service{j: j} }

// List returns the open project's kept jars (no values).
func (s *Service) List() ([]Jar, error) { return s.j.List() }

// Delete removes one cookie from file's jar.
func (s *Service) Delete(file, domain, cookiePath, name string) error {
	return s.j.Delete(file, domain, cookiePath, name)
}

// Clear empties file's jar, or every jar with file "".
func (s *Service) Clear(file string) error { return s.j.Clear(file) }
