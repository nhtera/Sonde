// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/value"
)

// Project edits change one variable of an environment in the file it
// comes from (its effective source, as VariableNames reports it): the
// inline "variables:" of sonde.yaml, a variables_files file or a
// secrets_files file. sonde.yaml is edited by splicing lines located with
// its YAML node tree, never re-encoded, so comments, anchors, order and
// line endings stay. Every edit is checked by loading a copy of the
// project with it and resolving every environment it does not break
// already.

// ErrEditByHand reports a sonde.yaml shape the editor does not change
// (flow style, multi-line values): the user edits it by hand.
var ErrEditByHand = errors.New("edit sonde.yaml by hand")

// FileEdit is the new content of one file of a project.
type FileEdit struct {
	// Path is the file's absolute path.
	Path string
	Data []byte
	// Perm is the permission of a created file: 0600 for a secrets file.
	Perm fs.FileMode
	// Created is set when the file does not exist yet.
	Created bool
}

// projectEdit accumulates the new content of the files of a project.
type projectEdit struct {
	p     *Project
	files map[string]*FileEdit // by path relative to the project
	order []string
}

func (p *Project) newEdit() *projectEdit {
	return &projectEdit{p: p, files: map[string]*FileEdit{}}
}

// read returns the current (possibly edited) content of rel.
func (e *projectEdit) read(rel string) ([]byte, bool, error) {
	if f, ok := e.files[rel]; ok {
		return f.Data, !f.Created, nil
	}
	root, err := newProjectSandbox(e.p.Dir)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	data, err := root.ReadFile(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// write records the new content of rel.
func (e *projectEdit) write(rel string, data []byte, perm fs.FileMode) error {
	if f, ok := e.files[rel]; ok {
		f.Data = data
		return nil
	}
	_, exists, err := e.read(rel)
	if err != nil {
		return err
	}
	e.files[rel] = &FileEdit{Path: filepath.Join(e.p.Dir, filepath.FromSlash(rel)), Data: data, Perm: perm, Created: !exists}
	e.order = append(e.order, rel)
	return nil
}

// result validates the edits and returns them in order.
func (e *projectEdit) result() ([]FileEdit, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	out := make([]FileEdit, 0, len(e.order))
	for _, rel := range e.order {
		out = append(out, *e.files[rel])
	}
	return out, nil
}

// yamlRel is sonde.yaml's path relative to the project.
func (e *projectEdit) yamlRel() string { return filepath.ToSlash(filepath.Base(e.p.Path)) }

// SetVariable sets variable name of environment env to v in its effective
// source; a new name is added to the inline "variables:" of env. A name
// that is a secret stays one, with v's text as its value.
func (p *Project) SetVariable(env, name string, v value.Value) ([]FileEdit, error) {
	names, err := p.VariableNames(env)
	if err != nil {
		return nil, err
	}
	e := p.newEdit()
	src, ok := names[name]
	switch {
	case ok && src.Secret:
		err = e.setProperty(src.File, name, value.Display(v), 0o600)
	case ok && src.File != "":
		var raw string
		if raw, err = PropertyText(v); err == nil {
			err = e.setProperty(src.File, name, raw, 0o644)
		}
	default:
		err = e.yamlSetVariable(env, name, v)
	}
	if err != nil {
		return nil, err
	}
	return e.result()
}

// SetSecret sets secret name of environment env to secret. A name that
// is already a secret is set in its file; any other moves to the secrets
// file env/<env>.secrets (created with permission 0600 and added to the
// environment's secrets_files when missing), and is removed from the
// inline variables and the variables files of env.
func (p *Project) SetSecret(env, name, secret string) ([]FileEdit, error) {
	names, err := p.VariableNames(env)
	if err != nil {
		return nil, err
	}
	e := p.newEdit()
	if src, ok := names[name]; ok && src.Secret {
		if err := e.setProperty(src.File, name, secret, 0o600); err != nil {
			return nil, err
		}
		return e.result()
	}
	rel, err := secretsFileFor(env)
	if err != nil {
		return nil, err
	}
	if err := e.setProperty(rel, name, secret, 0o600); err != nil {
		return nil, err
	}
	if !slices.Contains(p.Environments[env].SecretsFiles, rel) {
		if err := e.yamlAddSecretsFile(env, rel); err != nil {
			return nil, err
		}
	}
	if err := e.removeVariable(env, name, false); err != nil {
		return nil, err
	}
	return e.result()
}

// RemoveVariable removes name from every source of environment env.
func (p *Project) RemoveVariable(env, name string) ([]FileEdit, error) {
	if _, ok := p.Environments[env]; !ok {
		return nil, fmt.Errorf("%s: unknown environment %q (available: %s)", p.Path, env, strings.Join(p.envNames(), ", "))
	}
	e := p.newEdit()
	if err := e.removeVariable(env, name, true); err != nil {
		return nil, err
	}
	return e.result()
}

var envFileRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// secretsFileFor is the secrets file a new secret of env goes to.
func secretsFileFor(env string) (string, error) {
	if !envFileRE.MatchString(env) {
		return "", fmt.Errorf("environment %q: no secrets file can be named after it", env)
	}
	return "env/" + env + ".secrets", nil
}

// setProperty sets name to raw in the properties file rel.
func (e *projectEdit) setProperty(rel, name, raw string, perm fs.FileMode) error {
	data, _, err := e.read(rel)
	if err != nil {
		return err
	}
	out, err := SetProperty(data, name, raw)
	if err != nil {
		return err
	}
	return e.write(rel, out, perm)
}

// removeVariable removes name from the inline variables and the
// variables files of env, and from its secrets files when secrets is set.
func (e *projectEdit) removeVariable(env, name string, secrets bool) error {
	envDef := e.p.Environments[env]
	if err := e.yamlRemoveVariable(env, name); err != nil {
		return err
	}
	files := slices.Clone(envDef.VariablesFiles)
	if secrets {
		files = append(files, envDef.SecretsFiles...)
	}
	for _, rel := range files {
		data, exists, err := e.read(rel)
		if err != nil || !exists {
			continue // an unreadable file defines nothing to remove
		}
		if out := RemoveProperty(data, name); !bytes.Equal(out, data) {
			if err := e.write(rel, out, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// validate loads a copy of the project with the edits and resolves every
// environment.
func (e *projectEdit) validate() error {
	tmp, err := os.MkdirTemp("", "sonde-project-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // temporary copy
	yamlRel := e.yamlRel()
	yamlData, _, err := e.read(yamlRel)
	if err != nil {
		return err
	}
	if err := writeCopy(tmp, yamlRel, yamlData); err != nil {
		return err
	}
	copied, err := LoadProject(filepath.Join(tmp, yamlRel))
	if err != nil {
		return fmt.Errorf("the edit would break sonde.yaml: %w", err)
	}
	for _, env := range copied.Environments {
		for _, rel := range append(slices.Clone(env.VariablesFiles), env.SecretsFiles...) {
			data, exists, err := e.read(rel)
			if err != nil {
				return err
			}
			if exists || e.files[rel] != nil {
				if err := writeCopy(tmp, rel, data); err != nil {
					return err
				}
			}
		}
	}
	// An environment that did not resolve before (a secrets file not
	// checked out) does not block the edit; one the edit breaks does.
	for _, name := range copied.envNames() {
		if _, _, err := copied.Resolve(name); err != nil {
			if _, _, before := e.p.Resolve(name); before == nil {
				return fmt.Errorf("the edit would break environment %s: %w", name, err)
			}
		}
	}
	return nil
}

func writeCopy(dir, rel string, data []byte) error {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// yamlDoc is sonde.yaml's source and node tree.
type yamlDoc struct {
	src   []byte
	root  *yaml.Node // the top mapping
	lines []int      // byte offset of each line start
	nl    string
}

func (e *projectEdit) yamlLoad() (*yamlDoc, error) {
	src, _, err := e.read(e.yamlRel())
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, ErrEditByHand
	}
	y := &yamlDoc{src: src, root: doc.Content[0], nl: newline(src), lines: []int{0}}
	for i, b := range src {
		if b == '\n' {
			y.lines = append(y.lines, i+1)
		}
	}
	return y, nil
}

// save records the new sonde.yaml.
func (e *projectEdit) yamlSave(src []byte) error { return e.write(e.yamlRel(), src, 0o644) }

// get returns the key and value nodes of key in mapping m.
func get(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// offset is the byte offset of a node's line and (rune) column.
func (y *yamlDoc) offset(line, col int) int {
	if line-1 >= len(y.lines) {
		return len(y.src)
	}
	off := y.lines[line-1]
	for c := 1; c < col && off < len(y.src); c++ {
		_, size := utf8.DecodeRune(y.src[off:])
		off += size
	}
	return off
}

// lineEnd is the offset after the line ending of line (1-based).
func (y *yamlDoc) lineEnd(line int) int {
	if line < len(y.lines) {
		return y.lines[line]
	}
	return len(y.src)
}

// lastLine is the last line a node spans.
func lastLine(n *yaml.Node) int {
	last := n.Line
	if n.Kind == yaml.ScalarNode && n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		last += strings.Count(strings.TrimRight(n.Value, "\n"), "\n") + 1
	}
	for _, c := range n.Content {
		last = max(last, lastLine(c))
	}
	return last
}

// splice replaces src[start:end] with text.
func splice(src []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(src)+len(text))
	out = append(out, src[:start]...)
	out = append(out, text...)
	return append(out, src[end:]...)
}

// env returns env's mapping node, refusing a flow-style one.
func (y *yamlDoc) env(env string) (key, val *yaml.Node, err error) {
	_, envs := get(y.root, "environments")
	key, val = get(envs, env)
	if key == nil {
		return nil, nil, fmt.Errorf("unknown environment %q", env)
	}
	if envs.Style&yaml.FlowStyle != 0 || val.Style&yaml.FlowStyle != 0 {
		return nil, nil, ErrEditByHand
	}
	return key, val, nil
}

// indentOf is the indentation of the keys of mapping m, or of a new
// mapping under the key k.
func indentOf(m, k *yaml.Node) string {
	if m != nil && m.Kind == yaml.MappingNode && len(m.Content) > 0 {
		return strings.Repeat(" ", m.Content[0].Column-1)
	}
	return strings.Repeat(" ", k.Column-1+2)
}

// yamlScalar renders v as a YAML scalar that decodes (variableFromNode)
// back to v, in the style of the node it replaces (nil: a new one).
func yamlScalar(v value.Value, old *yaml.Node) (string, error) {
	var cands []string
	switch s := v.(type) {
	case value.String:
		if strings.ContainsAny(string(s), "\r\n") {
			return "", fmt.Errorf("%w: a multi-line value", ErrEditByHand)
		}
		plain, single, double := string(s), "'"+strings.ReplaceAll(string(s), "'", "''")+"'", strconv.Quote(string(s))
		switch {
		case old != nil && old.Style&yaml.SingleQuotedStyle != 0:
			cands = []string{single, double}
		case old != nil && old.Style&yaml.DoubleQuotedStyle != 0:
			cands = []string{double}
		default:
			cands = []string{plain, double}
		}
	case value.Bool, value.Int, value.BigInt, value.Float, value.Null:
		cands = []string{value.Display(v)}
		if f, ok := v.(value.Float); ok {
			cands = append(cands, strconv.FormatFloat(float64(f), 'g', -1, 64))
		}
	default:
		return "", fmt.Errorf("a %s can not be a sonde.yaml variable", v.Kind())
	}
	for _, c := range cands {
		var doc yaml.Node
		if yaml.Unmarshal([]byte("k: "+c+"\n"), &doc) != nil || len(doc.Content) != 1 {
			continue
		}
		_, n := get(doc.Content[0], "k")
		if n == nil {
			continue
		}
		if got, err := variableFromNode(n); err == nil && got.Kind() == v.Kind() && value.Equal(got, v) {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s can not be written as a sonde.yaml value", value.Repr(v))
}

// yamlKey renders a mapping key.
func yamlKey(name string) (string, error) {
	return yamlScalar(value.String(name), nil)
}

// yamlSetVariable sets an inline variable of env.
func (e *projectEdit) yamlSetVariable(env, name string, v value.Value) error {
	y, err := e.yamlLoad()
	if err != nil {
		return err
	}
	envKey, envVal, err := y.env(env)
	if err != nil {
		return err
	}
	varsKey, vars := get(envVal, "variables")
	if vars != nil && vars.Style&yaml.FlowStyle != 0 {
		return ErrEditByHand
	}
	if k, old := get(vars, name); k != nil {
		start, end, err := y.valueRange(k, old)
		if err != nil {
			return err
		}
		text, err := yamlScalar(v, old)
		if err != nil {
			return err
		}
		return e.yamlSave(splice(y.src, start, end, text))
	}
	key, err := yamlKey(name)
	if err != nil {
		return err
	}
	text, err := yamlScalar(v, nil)
	if err != nil {
		return err
	}
	if varsKey != nil {
		line := indentOf(vars, varsKey) + key + ": " + text + y.nl
		at := y.lineEnd(lastLine(varsKey))
		if vars.Kind == yaml.MappingNode {
			at = y.lineEnd(lastLine(vars))
		}
		return e.yamlSave(splice(y.src, at, at, y.lead(at)+line))
	}
	ind := indentOf(envVal, envKey)
	block := ind + "variables:" + y.nl + ind + "  " + key + ": " + text + y.nl
	at := y.lineEnd(lastLine(envKey))
	if envVal.Kind == yaml.MappingNode {
		at = y.lineEnd(lastLine(envVal))
	}
	return e.yamlSave(splice(y.src, at, at, y.lead(at)+block))
}

// lead is the line ending to write before an insertion at at, when the
// line before it has none (the end of the file).
func (y *yamlDoc) lead(at int) string {
	if at > 0 && y.src[at-1] != '\n' {
		return y.nl
	}
	return ""
}

// valueRange returns the byte range of a single-line scalar value (before
// a trailing comment).
func (y *yamlDoc) valueRange(k, v *yaml.Node) (int, int, error) {
	if v.Kind != yaml.ScalarNode || v.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || v.Line != k.Line || v.Alias != nil || v.Anchor != "" {
		return 0, 0, ErrEditByHand
	}
	start := y.offset(v.Line, v.Column)
	lineEnd := y.lineEnd(v.Line)
	rest := strings.TrimRight(string(y.src[start:lineEnd]), "\r\n")
	for _, c := range []string{v.LineComment, k.LineComment} {
		if c != "" {
			if i := strings.LastIndex(rest, c); i >= 0 {
				rest = rest[:i]
			}
		}
	}
	rest = strings.TrimRight(rest, " \t")
	// The range must hold exactly the value: a quoted value spanning
	// lines is left to the user.
	var doc yaml.Node
	if yaml.Unmarshal([]byte("k: "+rest+"\n"), &doc) != nil || len(doc.Content) != 1 {
		return 0, 0, ErrEditByHand
	}
	if _, n := get(doc.Content[0], "k"); n == nil || n.Value != v.Value || n.Tag != v.Tag {
		return 0, 0, ErrEditByHand
	}
	return start, start + len(rest), nil
}

// yamlRemoveVariable removes an inline variable of env, with its line.
func (e *projectEdit) yamlRemoveVariable(env, name string) error {
	y, err := e.yamlLoad()
	if err != nil {
		return err
	}
	_, envVal, err := y.env(env)
	if err != nil {
		return err
	}
	_, vars := get(envVal, "variables")
	k, v := get(vars, name)
	if k == nil {
		return nil
	}
	if vars.Style&yaml.FlowStyle != 0 {
		return ErrEditByHand
	}
	if _, _, err := y.valueRange(k, v); err != nil {
		return err
	}
	return e.yamlSave(splice(y.src, y.lines[k.Line-1], y.lineEnd(k.Line), ""))
}

// yamlAddSecretsFile appends rel to the secrets_files of env.
func (e *projectEdit) yamlAddSecretsFile(env, rel string) error {
	y, err := e.yamlLoad()
	if err != nil {
		return err
	}
	envKey, envVal, err := y.env(env)
	if err != nil {
		return err
	}
	item, err := yamlKey(rel)
	if err != nil {
		return err
	}
	k, seq := get(envVal, "secrets_files")
	switch {
	case k == nil:
		ind := indentOf(envVal, envKey)
		block := ind + "secrets_files:" + y.nl + ind + "  - " + item + y.nl
		at := y.lineEnd(lastLine(envKey))
		if envVal.Kind == yaml.MappingNode {
			at = y.lineEnd(lastLine(envVal))
		}
		return e.yamlSave(splice(y.src, at, at, y.lead(at)+block))
	case seq.Kind == yaml.SequenceNode && seq.Style&yaml.FlowStyle == 0 && len(seq.Content) > 0:
		first := seq.Content[0]
		line := strings.Repeat(" ", first.Column-3) + "- " + item + y.nl
		at := y.lineEnd(lastLine(seq))
		return e.yamlSave(splice(y.src, at, at, y.lead(at)+line))
	case seq.Kind == yaml.SequenceNode && seq.Style&yaml.FlowStyle != 0 && lastLine(seq) == seq.Line:
		// [a, b] on one line: insert before its closing bracket.
		lineText := string(y.src[y.lines[seq.Line-1]:y.lineEnd(seq.Line)])
		i := strings.LastIndex(lineText, "]")
		if i < 0 {
			return ErrEditByHand
		}
		sep := ", "
		if len(seq.Content) == 0 {
			sep = ""
		}
		at := y.lines[seq.Line-1] + i
		return e.yamlSave(splice(y.src, at, at, sep+item))
	}
	return ErrEditByHand
}
