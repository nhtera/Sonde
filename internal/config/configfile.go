// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileOptions are the options a config file may set.
type FileOptions struct {
	// Verbose is set true by a bare "--verbose" line.
	Verbose bool
	// Headers accumulates one entry per "--header" line, in file order.
	Headers []string
	// MaxRedirs is set by a "--max-redirs" line; nil means absent.
	MaxRedirs *int
	// UserAgent is set by a "--user-agent" line; nil means absent.
	UserAgent *string
}

// FilePath returns the config file path, in order of precedence
// $XDG_CONFIG_HOME/hurl/config if XDG_CONFIG_HOME is set, else
// $HOME/config/hurl/config if HOME is set (this second form reproduces the
// upstream 8.0.1 path exactly, including its "config" rather than
// ".config" component; sonde matches it for parity even though the
// upstream doc comment says ".config").
func (e Env) FilePath() (string, bool) {
	if dir, ok := e["XDG_CONFIG_HOME"]; ok {
		return filepath.Join(dir, "hurl", "config"), true
	}
	if home, ok := e["HOME"]; ok {
		return filepath.Join(home, "config", "hurl", "config"), true
	}
	return "", false
}

// configPos is a 1-based line/column position in a config file.
type configPos struct{ line, col int }

// configFileError is a config file parse error at a position; its
// Error() form ("line:col: message") matches the upstream parser's own,
// so LoadConfigFile's "path:line:col: message" reproduces it exactly.
type configFileError struct {
	pos configPos
	msg string
}

func newConfigFileError(pos configPos, msg string) *configFileError {
	return &configFileError{pos: pos, msg: msg}
}

func (e *configFileError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.pos.line, e.pos.col, e.msg)
}

// ParseFile parses config file content: lines of "--option[ value]" or
// "--option=value", blank lines, lines starting with '#' and horizontal
// whitespace ignored. Recognized options are --verbose (no value),
// --header, --max-redirs and --user-agent (values may be double-quoted,
// and a quoted value may itself span multiple lines). The parser mirrors
// the upstream implementation position for position, so a parse error's
// line:col matches it exactly.
func ParseFile(content string) (FileOptions, error) {
	var opts FileOptions
	r := newConfigReader(content)
	for !r.isEOF() {
		if err := parseConfigOption(r, &opts); err != nil {
			return FileOptions{}, err
		}
	}
	return opts, nil
}

// LoadConfigFile reads and parses the config file at path. An empty path,
// or a path that does not exist, returns zero FileOptions and no error.
func LoadConfigFile(path string) (FileOptions, error) {
	if path == "" {
		return FileOptions{}, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: a fixed, well-known path
	if errors.Is(err, os.ErrNotExist) {
		return FileOptions{}, nil
	}
	if err != nil {
		return FileOptions{}, fmt.Errorf("Failed to read config file %s: %w", path, err)
	}
	opts, err := ParseFile(string(data))
	if err != nil {
		return FileOptions{}, fmt.Errorf("%s:%s", path, err) //nolint:staticcheck,revive // kept for CLI message-format compatibility
	}
	return opts, nil
}

// parseConfigOption parses one "--option..." at the reader's current
// position, skipping any leading whitespace and comment lines first; it
// does nothing when that skip reaches EOF (a trailing blank/comment
// line).
func parseConfigOption(r *configReader, opts *FileOptions) error {
	skipConfigWhitespaceAndComments(r)
	if r.isEOF() {
		return nil
	}
	save := r.position()
	if r.readN(2) != "--" {
		return newConfigFileError(save, "Expecting an option starting with --")
	}
	name := r.readWhile(isConfigNameRune)
	switch name {
	case "verbose":
		if err := expectNoConfigValue(r); err != nil {
			return err
		}
		opts.Verbose = true
		return nil
	case "header":
		if err := parseConfigValueSeparator(r); err != nil {
			return err
		}
		value, err := parseConfigValue(r)
		if err != nil {
			return err
		}
		if value == "" {
			return newConfigFileError(save, "Option --header requires a value")
		}
		opts.Headers = append(opts.Headers, value)
		return nil
	case "max-redirs":
		if err := parseConfigValueSeparator(r); err != nil {
			return err
		}
		valuePos := r.position()
		value, err := parseConfigValue(r)
		if err != nil {
			return err
		}
		n, convErr := strconv.Atoi(value)
		if convErr != nil {
			return newConfigFileError(valuePos, "Option --max-redirs requires an integer value")
		}
		if n < -1 {
			return newConfigFileError(valuePos, "Option --max-redirs requires an integer value >= -1")
		}
		opts.MaxRedirs = &n
		return nil
	case "user-agent":
		if err := parseConfigValueSeparator(r); err != nil {
			return err
		}
		value, err := parseConfigValue(r)
		if err != nil {
			return err
		}
		opts.UserAgent = &value
		return nil
	default:
		return newConfigFileError(save, fmt.Sprintf("Unknown option <--%s>", name))
	}
}

// isConfigNameRune reports whether c can be part of an option name: a
// letter, digit, '-' or '_'.
func isConfigNameRune(c rune) bool {
	return c == '-' || c == '_' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// skipConfigWhitespaceAndComments skips whitespace (including newlines)
// and, when a line starts with '#' after that, the rest of the line too,
// repeating until neither applies.
func skipConfigWhitespaceAndComments(r *configReader) {
	for {
		r.readWhile(isConfigSpaceRune)
		if r.isEOF() {
			return
		}
		if c, _ := r.peek(); c != '#' {
			return
		}
		r.readWhile(func(c rune) bool { return c != '\n' })
		if c, ok := r.peek(); ok && c == '\n' {
			r.read()
		}
	}
}

// expectNoConfigValue requires the rest of the line to be blank or a
// comment (--verbose takes no value); it consumes through the line's own
// newline, so the reader ends up at the start of the next line.
func expectNoConfigValue(r *configReader) error {
	for {
		r.readWhile(isConfigHSpaceRune)
		if r.isEOF() {
			return nil
		}
		c, _ := r.peek()
		switch c {
		case '#':
			r.readWhile(func(c rune) bool { return c != '\n' })
			if c2, ok := r.peek(); ok && c2 == '\n' {
				r.read()
			}
		case '\n':
			r.read()
			return nil
		default:
			return newConfigFileError(r.position(), "Not expecting a value for this option")
		}
	}
}

// parseConfigValueSeparator consumes one or more spaces/tabs, or else a
// single '=', between an option name and its value.
func parseConfigValueSeparator(r *configReader) error {
	if r.readWhile(isConfigHSpaceRune) != "" {
		return nil
	}
	if c, ok := r.peek(); ok && c == '=' {
		r.read()
		return nil
	}
	return newConfigFileError(r.position(), "Expecting a value using space or '=' separator")
}

// parseConfigValue reads an option value: a double-quoted string (which
// may itself contain newlines, trimmed inside the quotes), or otherwise
// everything up to the end of the line, trimmed.
func parseConfigValue(r *configReader) (string, error) {
	if c, ok := r.peek(); ok && c == '"' {
		r.read()
		value := r.readWhile(func(c rune) bool { return c != '"' })
		if c2, ok := r.peek(); ok && c2 == '"' {
			r.read()
			return strings.TrimSpace(value), nil
		}
		return "", newConfigFileError(r.position(), "Missing closing quote")
	}
	return strings.TrimSpace(r.readWhile(func(c rune) bool { return c != '\n' })), nil
}

func isConfigSpaceRune(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isConfigHSpaceRune(c rune) bool {
	return c == ' ' || c == '\t'
}

// configReader is a rune reader over config file content that tracks its
// 1-based line/column position, so a parse error's position matches the
// upstream reader-based parser exactly.
type configReader struct {
	runes []rune
	pos   int
	line  int
	col   int
}

func newConfigReader(s string) *configReader {
	return &configReader{runes: []rune(s), line: 1, col: 1}
}

func (r *configReader) isEOF() bool { return r.pos >= len(r.runes) }

func (r *configReader) position() configPos { return configPos{r.line, r.col} }

func (r *configReader) peek() (rune, bool) {
	if r.isEOF() {
		return 0, false
	}
	return r.runes[r.pos], true
}

func (r *configReader) read() (rune, bool) {
	c, ok := r.peek()
	if !ok {
		return 0, false
	}
	r.pos++
	if c == '\n' {
		r.line++
		r.col = 1
	} else {
		r.col++
	}
	return c, true
}

// readN reads up to n runes, stopping early at EOF.
func (r *configReader) readN(n int) string {
	start := r.pos
	for range n {
		if _, ok := r.read(); !ok {
			break
		}
	}
	return string(r.runes[start:r.pos])
}

// readWhile reads runes while pred holds, stopping at EOF or the first
// rune pred rejects.
func (r *configReader) readWhile(pred func(rune) bool) string {
	start := r.pos
	for {
		c, ok := r.peek()
		if !ok || !pred(c) {
			break
		}
		r.read()
	}
	return string(r.runes[start:r.pos])
}
