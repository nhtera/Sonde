// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// FileOptions are the options a config file sets, typed and checked
// when the file is read. A pointer, or an empty string, is an option the
// file does not set.
type FileOptions struct {
	// Path is the file the options were read from ("" when none was).
	Path string
	// Keys are the options the file sets, without "--", in the order they
	// first appear.
	Keys []string

	// Color is set by --color (true) and --no-color (false): the last
	// one wins.
	Color           *bool
	Compressed      bool
	ConnectTimeout  *time.Duration
	ContinueOnError bool
	Delay           *time.Duration
	// ErrorFormat is "short" or "long".
	ErrorFormat  string
	FailWithBody bool
	// Headers, NoHeaders and ProxyHeaders accumulate one entry per line,
	// in file order.
	Headers []string
	// HTTPVersion is "1.0", "1.1", "2" or "3" (--http1.0 ... --http3):
	// the last one wins.
	HTTPVersion        string
	Insecure           bool
	IPv6               bool
	Jobs               *int
	LimitRate          *int64
	Location           bool
	LocationTrusted    bool
	MaxFilesize        *int64
	MaxRedirs          *int
	MaxTime            *time.Duration
	NoAssert           bool
	NoCookieStore      bool
	NoHeaders          []string
	NoJSONPathCoercion bool
	NoOutput           bool
	NoProgressBar      bool
	NoProxy            *string
	// Pretty is set by --pretty (true) and --no-pretty (false): the last
	// one wins.
	Pretty        *bool
	Proxy         *string
	ProxyHeaders  []string
	Retry         *int
	RetryInterval *time.Duration
	// Secrets are the --secret NAME=VALUE lines; a name can be given once.
	Secrets   map[string]string
	Test      bool
	User      *string
	UserAgent *string
	// Variables are the --variable NAME=VALUE lines, in file order (a
	// later line for a name replaces an earlier one).
	Variables []Assignment
	// Verbosity is "brief", "verbose" or "debug" (--verbose is
	// "verbose", --very-verbose "debug").
	Verbosity string
}

// Sets reports whether the file sets option key (without "--").
func (o FileOptions) Sets(key string) bool { return slices.Contains(o.Keys, key) }

// FilePath returns the config file path: $XDG_CONFIG_HOME/hurl/config
// when XDG_CONFIG_HOME is set, else $HOME/.config/hurl/config when HOME
// is.
// An empty variable counts as unset (the XDG rule), so it never selects
// ./hurl/config in the working directory.
func (e Env) FilePath() (string, bool) {
	if dir := e["XDG_CONFIG_HOME"]; dir != "" {
		return filepath.Join(dir, "hurl", "config"), true
	}
	if home := e["HOME"]; home != "" {
		return filepath.Join(home, ".config", "hurl", "config"), true
	}
	return "", false
}

// LegacyFilePath is where the reference implementation 8.0.1 looked when
// XDG_CONFIG_HOME is unset ($HOME/config/hurl/config, without the dot);
// "" when that does not apply.
func (e Env) LegacyFilePath() string {
	if e["XDG_CONFIG_HOME"] != "" || e["HOME"] == "" {
		return ""
	}
	return filepath.Join(e["HOME"], "config", "hurl", "config")
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

// ParseFile parses config file content: one "--option" per line, its
// value (if it takes one) after spaces or '=', blank lines and '#'
// comments ignored. A value may be double-quoted, and a quoted value may
// span lines. Every option is checked as it is read, so an error names
// the position the upstream parser reports, with its wording.
//
// Unlike the upstream parser, a leading UTF-8 BOM is ignored and a CRLF
// line end is read as a newline.
func ParseFile(content string) (FileOptions, error) {
	var opts FileOptions
	r := newConfigReader(strings.TrimPrefix(content, "\uFEFF"))
	for !r.isEOF() {
		if err := parseConfigOption(r, &opts); err != nil {
			return FileOptions{}, err
		}
	}
	return opts, nil
}

// LoadConfigFile reads and parses the config file at path. An empty path,
// or a path that cannot be seen (missing, or in a directory that cannot be
// searched), returns zero FileOptions and no error.
func LoadConfigFile(path string) (FileOptions, error) {
	if path == "" {
		return FileOptions{}, nil
	}
	if _, err := os.Stat(path); err != nil {
		return FileOptions{}, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: a fixed, well-known path
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return FileOptions{}, fmt.Errorf("Failed to read config file %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return FileOptions{}, fmt.Errorf("Failed to read config file %s: stream did not contain valid UTF-8", path)
	}
	opts, err := ParseFile(string(data))
	if err != nil {
		return FileOptions{}, fmt.Errorf("%s:%s", path, err) //nolint:staticcheck,revive // kept for CLI message-format compatibility
	}
	opts.Path = path
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
	optionPos := r.position()
	if r.readN(2) != "--" {
		return newConfigFileError(optionPos, "Expecting an option starting with --")
	}
	name := r.readWhile(isConfigNameRune)
	parse, ok := configOptions[name]
	if !ok {
		return newConfigFileError(optionPos, fmt.Sprintf("Unknown option <--%s>", name))
	}
	if err := parse(r, opts, optionPos); err != nil {
		return err
	}
	if !opts.Sets(name) {
		opts.Keys = append(opts.Keys, name)
	}
	return nil
}

// configOption parses one option's value (if any) into opts; optionPos
// is where its "--" starts.
type configOption func(r *configReader, opts *FileOptions, optionPos configPos) error

// configOptions are the options a config file accepts, as the upstream
// parser at 498d4a4f (packages/hurl/src/cli/options/config_file/mod.rs)
// lists them, with its per-option messages.
var configOptions = map[string]configOption{
	"color":             flagOption(func(o *FileOptions) { o.Color = ptr(true) }),
	"compressed":        flagOption(func(o *FileOptions) { o.Compressed = true }),
	"connect-timeout":   durationOption("connect-timeout", Second, func(o *FileOptions, d time.Duration) { o.ConnectTimeout = &d }),
	"continue-on-error": flagOption(func(o *FileOptions) { o.ContinueOnError = true }),
	"delay":             durationOption("delay", Millisecond, func(o *FileOptions, d time.Duration) { o.Delay = &d }),
	"error-format": valueOption(func(o *FileOptions, v string) string {
		if v != "short" && v != "long" {
			return "Option --error-format requires one of the following values: short, long"
		}
		o.ErrorFormat = v
		return ""
	}),
	"fail-with-body": flagOption(func(o *FileOptions) { o.FailWithBody = true }),
	"header":         requiredOption("header", func(o *FileOptions, v string) { o.Headers = append(o.Headers, v) }),
	"http1.0":        flagOption(func(o *FileOptions) { o.HTTPVersion = "1.0" }),
	"http1.1":        flagOption(func(o *FileOptions) { o.HTTPVersion = "1.1" }),
	"http2":          flagOption(func(o *FileOptions) { o.HTTPVersion = "2" }),
	"http3":          flagOption(func(o *FileOptions) { o.HTTPVersion = "3" }),
	"insecure":       flagOption(func(o *FileOptions) { o.Insecure = true }),
	"ipv6":           flagOption(func(o *FileOptions) { o.IPv6 = true }),
	"jobs": valueOption(func(o *FileOptions, v string) string {
		n, err := parseUnsigned(v)
		if err != nil || n > math.MaxInt {
			return "Option --jobs requires an integer value"
		}
		if n < 1 {
			return "Option --jobs requires an integer value >= 1"
		}
		o.Jobs = ptr(int(n))
		return ""
	}),
	"limit-rate":       bytesOption("limit-rate", func(o *FileOptions, n int64) { o.LimitRate = &n }),
	"location":         flagOption(func(o *FileOptions) { o.Location = true }),
	"location-trusted": flagOption(func(o *FileOptions) { o.Location, o.LocationTrusted = true, true }),
	"max-filesize":     bytesOption("max-filesize", func(o *FileOptions, n int64) { o.MaxFilesize = &n }),
	"max-redirs": countOption("max-redirs", "Option --max-redirs requires an integer value >= -1",
		func(o *FileOptions, n int) { o.MaxRedirs = &n }),
	"max-time":             durationOption("max-time", Second, func(o *FileOptions, d time.Duration) { o.MaxTime = &d }),
	"no-assert":            flagOption(func(o *FileOptions) { o.NoAssert = true }),
	"no-color":             flagOption(func(o *FileOptions) { o.Color = ptr(false) }),
	"no-cookie-store":      flagOption(func(o *FileOptions) { o.NoCookieStore = true }),
	"no-header":            requiredOption("no-header", func(o *FileOptions, v string) { o.NoHeaders = append(o.NoHeaders, v) }),
	"no-jsonpath-coercion": flagOption(func(o *FileOptions) { o.NoJSONPathCoercion = true }),
	"no-output":            flagOption(func(o *FileOptions) { o.NoOutput = true }),
	"no-pretty":            flagOption(func(o *FileOptions) { o.Pretty = ptr(false) }),
	"no-progress-bar":      flagOption(func(o *FileOptions) { o.NoProgressBar = true }),
	"no-proxy":             requiredOption("no-proxy", func(o *FileOptions, v string) { o.NoProxy = &v }),
	"pretty":               flagOption(func(o *FileOptions) { o.Pretty = ptr(true) }),
	"proxy":                requiredOption("proxy", func(o *FileOptions, v string) { o.Proxy = &v }),
	"proxy-header":         requiredOption("proxy-header", func(o *FileOptions, v string) { o.ProxyHeaders = append(o.ProxyHeaders, v) }),
	// The upstream message says ">= 1" although -1 (forever) and 0 are
	// accepted: the range is the one of every count option.
	"retry": countOption("retry", "Option --retry requires an integer value >= 1",
		func(o *FileOptions, n int) { o.Retry = &n }),
	"retry-interval": durationOption("retry-interval", Millisecond, func(o *FileOptions, d time.Duration) { o.RetryInterval = &d }),
	"secret": valueOption(func(o *FileOptions, v string) string {
		a, err := ParseAssignment(v, Forced)
		if errors.Is(err, errMissingAssignmentValue) {
			// Never echo the text back: it may be the secret itself.
			return "invalid secret assignment: missing '='"
		}
		if err != nil {
			return err.Error()
		}
		if o.Secrets == nil {
			o.Secrets = map[string]string{}
		}
		if err := AddSecret(o.Secrets, a.Name, a.Value); err != nil {
			return err.Error()
		}
		return ""
	}),
	"test":       flagOption(func(o *FileOptions) { o.Test = true }),
	"user":       requiredOption("user", func(o *FileOptions, v string) { o.User = &v }),
	"user-agent": valueOption(func(o *FileOptions, v string) string { o.UserAgent = &v; return "" }),
	"variable": valueOption(func(o *FileOptions, v string) string {
		a, err := ParseAssignment(v, Inferred)
		if err != nil {
			return err.Error()
		}
		o.Variables = append(o.Variables, a)
		return ""
	}),
	"verbose":      flagOption(func(o *FileOptions) { o.Verbosity = "verbose" }),
	"very-verbose": flagOption(func(o *FileOptions) { o.Verbosity = "debug" }),
	"verbosity": valueOption(func(o *FileOptions, v string) string {
		if v != "brief" && v != "verbose" && v != "debug" {
			return "Option --verbosity requires one of the following values: brief, verbose, debug"
		}
		o.Verbosity = v
		return ""
	}),
}

// flagOption is an option that takes no value.
func flagOption(set func(*FileOptions)) configOption {
	return func(r *configReader, opts *FileOptions, _ configPos) error {
		if err := expectNoConfigValue(r); err != nil {
			return err
		}
		set(opts)
		return nil
	}
}

// valueOption is an option with a value: set returns the message for a
// value it rejects, reported at the value's position.
func valueOption(set func(*FileOptions, string) string) configOption {
	return func(r *configReader, opts *FileOptions, _ configPos) error {
		if err := parseConfigValueSeparator(r); err != nil {
			return err
		}
		valuePos := r.position()
		v, err := parseConfigValue(r)
		if err != nil {
			return err
		}
		if msg := set(opts, v); msg != "" {
			return newConfigFileError(valuePos, msg)
		}
		return nil
	}
}

// requiredOption is an option whose value cannot be empty; that error is
// reported at the option's position.
func requiredOption(name string, set func(*FileOptions, string)) configOption {
	return func(r *configReader, opts *FileOptions, optionPos configPos) error {
		if err := parseConfigValueSeparator(r); err != nil {
			return err
		}
		v, err := parseConfigValue(r)
		if err != nil {
			return err
		}
		if v == "" {
			return newConfigFileError(optionPos, "Option --"+name+" requires a value")
		}
		set(opts, v)
		return nil
	}
}

// durationOption is a duration option, a bare number being in unit.
func durationOption(name string, unit DurationUnit, set func(*FileOptions, time.Duration)) configOption {
	return valueOption(func(o *FileOptions, v string) string {
		d, err := ParseDuration(v, unit)
		if err != nil {
			return "Option --" + name + " has an invalid duration"
		}
		set(o, d)
		return ""
	})
}

// bytesOption is a non-negative integer option (a size or a rate).
func bytesOption(name string, set func(*FileOptions, int64)) configOption {
	return valueOption(func(o *FileOptions, v string) string {
		n, err := parseUnsigned(v)
		if err != nil || n > math.MaxInt64 {
			return "Option --" + name + " requires an integer value"
		}
		set(o, int64(n))
		return ""
	})
}

// countOption is a 32-bit integer option no less than -1 (unlimited);
// rangeMsg is the message for a smaller one.
func countOption(name, rangeMsg string, set func(*FileOptions, int)) configOption {
	return valueOption(func(o *FileOptions, v string) string {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return "Option --" + name + " requires an integer value"
		}
		if n < -1 {
			return rangeMsg
		}
		set(o, int(n))
		return ""
	})
}

// parseUnsigned parses a non-negative integer, accepting a leading '+'
// as the upstream parser does.
func parseUnsigned(s string) (uint64, error) {
	return strconv.ParseUint(strings.TrimPrefix(s, "+"), 10, 64)
}

func ptr[T any](v T) *T { return &v }

// isConfigNameRune reports whether c can be part of an option name: a
// letter, a digit, '-', '_' or '.' (--http1.1).
func isConfigNameRune(c rune) bool {
	return c == '-' || c == '_' || c == '.' || unicode.IsLetter(c) || unicode.IsNumber(c) ||
		unicode.Is(unicode.Other_Alphabetic, c)
}

// skipConfigWhitespaceAndComments skips whitespace (including newlines)
// and, when a line starts with '#' after that, the rest of the line too,
// repeating until neither applies.
func skipConfigWhitespaceAndComments(r *configReader) {
	for {
		r.readWhile(unicode.IsSpace)
		if r.isEOF() {
			return
		}
		if c, _ := r.peek(); c != '#' {
			return
		}
		skipConfigComment(r)
	}
}

// skipConfigComment skips the rest of a '#' line, its newline included.
func skipConfigComment(r *configReader) {
	r.readWhile(func(c rune) bool { return c != '\n' })
	if c, ok := r.peek(); ok && c == '\n' {
		r.read()
	}
}

// expectNoConfigValue requires the rest of the line to be blank or a
// comment (a flag takes no value); it consumes through the line's own
// newline, so the reader ends up at the start of the next line. (The
// upstream parser goes on reading after a comment, so it rejects the
// option on the next line; sonde stops at the comment's newline.)
func expectNoConfigValue(r *configReader) error {
	r.readWhile(isConfigHSpaceRune)
	c, ok := r.peek()
	switch {
	case !ok:
	case c == '#':
		skipConfigComment(r)
	case readConfigNewline(r):
	default:
		return newConfigFileError(r.position(), "Not expecting a value for this option")
	}
	return nil
}

// readConfigNewline consumes a "\n" or "\r\n" line end, reporting whether
// there was one.
func readConfigNewline(r *configReader) bool {
	if c, ok := r.peek(); ok && c == '\r' && r.pos+1 < len(r.runes) && r.runes[r.pos+1] == '\n' {
		r.read()
	}
	if c, ok := r.peek(); ok && c == '\n' {
		r.read()
		return true
	}
	return false
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

// parseConfigValue reads an option value, through the end of its line: a
// double-quoted string (which may itself contain newlines, trimmed inside
// the quotes, and may only be followed by blanks or a comment), or
// otherwise everything up to the end of the line, trimmed.
func parseConfigValue(r *configReader) (string, error) {
	if c, ok := r.peek(); !ok || c != '"' {
		value := strings.TrimSpace(r.readWhile(func(c rune) bool { return c != '\n' }))
		if c, ok := r.peek(); ok && c == '\n' {
			r.read()
		}
		return value, nil
	}
	r.read()
	value := strings.TrimSpace(r.readWhile(func(c rune) bool { return c != '"' }))
	if c, ok := r.peek(); !ok || c != '"' {
		return "", newConfigFileError(r.position(), "Missing closing quote")
	}
	r.read()
	r.readWhile(isConfigHSpaceRune)
	c, ok := r.peek()
	switch {
	case !ok:
	case c == '#':
		skipConfigComment(r)
	case readConfigNewline(r):
	default:
		return "", newConfigFileError(r.position(), "characters after the closing quote")
	}
	return value, nil
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
	switch {
	case c == '\n':
		r.line++
		r.col = 1
	case c >= '\u0301' && c <= '\u036e':
		// A combining mark takes no column of its own, as in the upstream
		// reader (and internal/syntax's port of it).
	default:
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
