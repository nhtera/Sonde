// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// This file recognizes shell parameter expansions ($NAME and ${NAME},
// NAME matching [A-Za-z_][A-Za-z0-9_]*) in unquoted and double-quoted
// text and turns them into a Sonde variable, so a real-world header like
// -H "Authorization: Bearer $TOKEN" imports with a working {{token}}
// placeholder instead of the literal text "$TOKEN". Single-quoted text,
// $'...' (ANSI-C quoting: see tokenizer.go's scanANSIC) and an escaped
// \$ are never scanned here — the tokenizer never calls into this file
// for them — so they always stay literal, matching a real shell. Any
// other '$'/backtick shell substitution ($(...), a backtick command
// substitution, a positional or special parameter like $1/$?/$@, or a
// ${...} with anything but a bare name) is never evaluated: it is left
// as literal text, exactly like an unrecognized one would be anyway, and
// reported once (per distinct snippet) as a warning.

// wordPart is one piece of a templatedString: literal text, or a
// $name/${name} expansion (name non-empty; lit unused).
type wordPart struct {
	lit  string
	name string
}

// templatedString is one shell word (or a value built from one) as
// literal text interspersed with parameter expansions. Every operation
// on it treats an expansion segment as one opaque unit: none of them
// ever look inside, split, or transform lit differently for name != ""
// runs, so an expansion always ends up as a real {{name}} placeholder in
// the generated file, never partially escaped or percent-encoded text.
type templatedString struct {
	parts []wordPart
}

// litString is a templatedString holding only literal text.
func litString(s string) templatedString {
	if s == "" {
		return templatedString{}
	}
	return templatedString{parts: []wordPart{{lit: s}}}
}

// isEmpty reports whether s has no content at all (not even an empty
// literal run).
func (s templatedString) isEmpty() bool { return len(s.parts) == 0 }

// firstLitOrEmpty returns s's first part's literal text, or "" if s is
// empty or starts with an expansion. Used only for combined short flags
// and --flag=value splitting, where the flag syntax itself is always
// literal text at the very start of a word.
func (s templatedString) firstLitOrEmpty() string {
	if len(s.parts) == 0 || s.parts[0].name != "" {
		return ""
	}
	return s.parts[0].lit
}

// tailFromFirstLit returns the templatedString starting at byte offset
// within s's first literal part (offset must be <= its length: the
// caller found it by scanning that same literal text), followed by every
// remaining part unchanged.
func (s templatedString) tailFromFirstLit(offset int) templatedString {
	if len(s.parts) == 0 {
		return templatedString{}
	}
	rest := append([]wordPart{{lit: s.parts[0].lit[offset:]}}, s.parts[1:]...)
	return templatedString{parts: rest}.normalized()
}

// text flattens s to a plain string, rendering an expansion as its real
// {{name}} display form (convert.VariableName, exactly as toText's Var
// segment would spell it): used for structural comparisons (a command
// name, a flag) that must never depend on unresolved template content,
// for matching against a fixed literal like "curl" or "@" that could
// never legitimately be split across an expansion boundary, and for a
// value with no typed-Text builder of its own (a curl -X method, a
// generic -d body — see entry.go) where embedding this same {{name}}
// text is the closest available representation.
func (s templatedString) text() string {
	if len(s.parts) == 1 && s.parts[0].name == "" {
		return s.parts[0].lit // the overwhelmingly common case
	}
	var b strings.Builder
	for _, p := range s.parts {
		if p.name != "" {
			b.WriteString("{{")
			b.WriteString(convert.VariableName(p.name))
			b.WriteString("}}")
			continue
		}
		b.WriteString(p.lit)
	}
	return b.String()
}

// hasExpansion reports whether s has at least one $NAME/${NAME} segment.
func (s templatedString) hasExpansion() bool {
	for _, p := range s.parts {
		if p.name != "" {
			return true
		}
	}
	return false
}

// splitTemplated splits s on every occurrence of sep found in a literal
// run, like strings.Split; an expansion can never contain sep (see cut).
func splitTemplated(s templatedString, sep string) []templatedString {
	var out []templatedString
	for {
		before, after, ok := s.cut(sep)
		if !ok {
			return append(out, s)
		}
		out = append(out, before)
		s = after
	}
}

// toText builds the syntax.Text this templatedString renders as: literal
// runs as-is, each expansion as a real {{name}} variable reference
// (convert.VariableName sanitizes name the same way every other importer
// does).
func (s templatedString) toText() syntax.Text {
	var t syntax.Text
	for _, p := range s.parts {
		if p.name != "" {
			t = append(t, syntax.Var(convert.VariableName(p.name)))
			continue
		}
		if p.lit != "" {
			t = append(t, syntax.Lit(p.lit))
		}
	}
	return t
}

// cutPrefix removes a literal prefix from the very start of s, reporting
// whether it was there. The prefix can only match within s's first
// literal run (a real curl flag prefix — "--", "@", "-" — is never split
// across an expansion boundary in practice); if s starts with an
// expansion instead, cutPrefix never matches.
func (s templatedString) cutPrefix(prefix string) (rest templatedString, ok bool) {
	if len(s.parts) == 0 || s.parts[0].name != "" {
		return s, false
	}
	lit, ok := strings.CutPrefix(s.parts[0].lit, prefix)
	if !ok {
		return s, false
	}
	return s.withFirstLit(lit), true
}

// trimSpace trims leading/trailing ASCII space and tab from s's first and
// last literal runs (curl's own -H value trimming never needs to trim
// around an expansion itself).
func (s templatedString) trimSpace() templatedString {
	out := s
	if len(out.parts) > 0 && out.parts[0].name == "" {
		out = out.withFirstLit(strings.TrimLeft(out.parts[0].lit, " \t"))
	}
	if n := len(out.parts); n > 0 && out.parts[n-1].name == "" {
		out = out.withLastLit(strings.TrimRight(out.parts[n-1].lit, " \t"))
	}
	return out
}

// contains reports whether any literal run of s contains sub (an
// expansion is opaque and never matches, matching how a real "@" or "="
// marker in curl's own mini-syntaxes is never produced by a variable's
// value).
func (s templatedString) contains(sub string) bool {
	for _, p := range s.parts {
		if p.name == "" && strings.Contains(p.lit, sub) {
			return true
		}
	}
	return false
}

// cut splits s at the first occurrence of sep found in any literal run,
// like strings.Cut; an expansion can never contain sep, so before/after
// split exactly at that point. ok is false when sep appears in no
// literal run at all.
func (s templatedString) cut(sep string) (before, after templatedString, ok bool) {
	for i, p := range s.parts {
		if p.name != "" {
			continue
		}
		if idx := strings.Index(p.lit, sep); idx >= 0 {
			before = templatedString{parts: append(append([]wordPart{}, s.parts[:i]...), wordPart{lit: p.lit[:idx]})}
			after = templatedString{parts: append([]wordPart{{lit: p.lit[idx+len(sep):]}}, s.parts[i+1:]...)}
			return before.normalized(), after.normalized(), true
		}
	}
	// Not found: match strings.Cut's own contract (before = s, after = "").
	return s, templatedString{}, false
}

// cutFirstOf splits s at the first occurrence, in any literal run, of
// whichever single byte in seps comes first (like strings.IndexAny,
// restricted to seps that are all one byte each — every caller here
// splits on "=" and/or "@", curl's own --data-urlencode and -F
// delimiters). ok is false when none of seps appears in any literal run.
func (s templatedString) cutFirstOf(seps string) (sep byte, before, after templatedString, ok bool) {
	for i, p := range s.parts {
		if p.name != "" {
			continue
		}
		if idx := strings.IndexAny(p.lit, seps); idx >= 0 {
			before = templatedString{parts: append(append([]wordPart{}, s.parts[:i]...), wordPart{lit: p.lit[:idx]})}
			after = templatedString{parts: append([]wordPart{{lit: p.lit[idx+1:]}}, s.parts[i+1:]...)}
			return p.lit[idx], before.normalized(), after.normalized(), true
		}
	}
	return 0, s, templatedString{}, false
}

// concat appends suffix (a literal string) to s.
func (s templatedString) concat(suffix string) templatedString {
	if suffix == "" {
		return s
	}
	if n := len(s.parts); n > 0 && s.parts[n-1].name == "" {
		return s.withLastLit(s.parts[n-1].lit + suffix)
	}
	return templatedString{parts: append(append([]wordPart{}, s.parts...), wordPart{lit: suffix})}
}

// add appends other to s.
func (s templatedString) add(other templatedString) templatedString {
	return templatedString{parts: append(append([]wordPart{}, s.parts...), other.parts...)}.normalized()
}

// join concatenates list with a literal separator between each element,
// like strings.Join.
func joinTemplated(list []templatedString, sep string) templatedString {
	var out templatedString
	for i, s := range list {
		if i > 0 {
			out = out.concat(sep)
		}
		out = out.add(s)
	}
	return out
}

// percentEncodeTemplated is percentEncode, applied only to s's literal
// runs: an expansion's eventual value is never percent-encoded twice
// (curl's own --data-urlencode never re-encodes what the shell already
// expanded either — it encodes the argument text as given, and a
// variable reference has no "argument text" of its own to encode).
func percentEncodeTemplated(s templatedString) templatedString {
	parts := make([]wordPart, len(s.parts))
	for i, p := range s.parts {
		if p.name != "" {
			parts[i] = p
			continue
		}
		parts[i] = wordPart{lit: percentEncode(p.lit)}
	}
	return templatedString{parts: parts}.normalized()
}

// withFirstLit returns s with its first part's literal text replaced (s
// must have a literal first part).
func (s templatedString) withFirstLit(lit string) templatedString {
	parts := append([]wordPart{}, s.parts...)
	parts[0] = wordPart{lit: lit}
	return templatedString{parts: parts}.normalized()
}

// withLastLit returns s with its last part's literal text replaced (s
// must have a literal last part).
func (s templatedString) withLastLit(lit string) templatedString {
	parts := append([]wordPart{}, s.parts...)
	parts[len(parts)-1] = wordPart{lit: lit}
	return templatedString{parts: parts}.normalized()
}

// normalized drops any empty literal part (concat/cut can produce one at
// a boundary), keeping the representation canonical.
func (s templatedString) normalized() templatedString {
	out := s.parts[:0:0]
	for _, p := range s.parts {
		if p.name == "" && p.lit == "" {
			continue
		}
		out = append(out, p)
	}
	return templatedString{parts: out}
}

// wordBuilder accumulates one word's text into a templatedString as the
// tokenizer scans it: literal bytes append to the in-progress run;
// appendVar flushes that run and adds a variable segment.
type wordBuilder struct {
	parts []wordPart
	lit   []byte
}

func (b *wordBuilder) appendByte(c byte)    { b.lit = append(b.lit, c) }
func (b *wordBuilder) appendBytes(s []byte) { b.lit = append(b.lit, s...) }

// isDigitsOnly reports whether b holds only ASCII digits so far, with no
// expansion segment: used to recognize a shell redirect's leading file
// descriptor number (e.g. the "2" of "2>&1"), so tokenize can discard it
// along with the redirect itself rather than flushing it as a stray word
// (see tokenize's '>' case, M2 in the phase 8 review).
func (b *wordBuilder) isDigitsOnly() bool {
	if len(b.parts) != 0 || len(b.lit) == 0 {
		return false
	}
	for _, c := range b.lit {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (b *wordBuilder) appendVar(name string) {
	b.flushLit()
	b.parts = append(b.parts, wordPart{name: name})
}

func (b *wordBuilder) flushLit() {
	if len(b.lit) > 0 {
		b.parts = append(b.parts, wordPart{lit: string(b.lit)})
		b.lit = b.lit[:0]
	}
}

// build finalizes b into a templatedString, ready for the next word.
func (b *wordBuilder) build() templatedString {
	b.flushLit()
	s := templatedString{parts: b.parts}
	*b = wordBuilder{}
	return s
}

// isNameStart/isNameChar match a shell parameter name:
// [A-Za-z_][A-Za-z0-9_]*.
func isNameStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNameChar(c byte) bool {
	return isNameStart(c) || c >= '0' && c <= '9'
}

// isSimpleName reports whether s (the content between "${" and "}") is
// exactly one parameter name, with nothing else — not a default value
// (${X:-y}), a length (${#X}), a substitution ${X/a/b}) or any other
// modifier.
func isSimpleName(s []byte) bool {
	if len(s) == 0 || !isNameStart(s[0]) {
		return false
	}
	for _, c := range s[1:] {
		if !isNameChar(c) {
			return false
		}
	}
	return true
}

// maxDollarScanAhead bounds how far scanDollar and dollarSnippet search
// forward for a matching '}' or ')': without this, an input built from
// many consecutive unterminated "$(" or "${" runs followed by one closer
// far away makes every one of them re-scan almost the whole remaining
// input for that same closer, which is O(N²) time (and, since each full
// span is captured as its own snippet, O(N²) memory too — see C1 in the
// phase 8 review, reproduced with "$(" repeated 5000+ times before a
// single ")"). Capping the search means a $(...) or ${...} longer than
// this is simply reported as unterminated (the "$" alone, literal) rather
// than matched across the whole file; real commands never come close to
// this bound.
const maxDollarScanAhead = 256

// maxSnippetLen caps how much of an unevaluated expansion's source text is
// kept for its warning message, so even a single long snippet within the
// maxDollarScanAhead bound can't produce an unbounded amount of warning
// text.
const maxSnippetLen = 64

// maxUnevaluatedWarnings caps how many distinct "shell expansion ... is
// not evaluated" warnings one input can produce; beyond it, Import adds a
// single "and N more" summary instead of one warning per remaining
// distinct snippet (see C1).
const maxUnevaluatedWarnings = 20

// dollarScan is the running state scanDollar shares across a whole
// tokenize call, so "one warning per distinct name" (and per distinct
// unevaluated snippet) holds across every word and every command of one
// input, not just within one word.
type dollarScan struct {
	names            []string
	seenName         map[string]bool
	unevaluated      []string
	seenOther        map[string]bool
	unevaluatedExtra int // count of distinct snippets dropped past maxUnevaluatedWarnings
}

func newDollarScan() *dollarScan {
	return &dollarScan{seenName: map[string]bool{}, seenOther: map[string]bool{}}
}

func (d *dollarScan) addName(name string) {
	if !d.seenName[name] {
		d.seenName[name] = true
		d.names = append(d.names, name)
	}
}

func (d *dollarScan) addUnevaluated(snippet string) {
	snippet = truncateSnippet(snippet)
	if d.seenOther[snippet] {
		return
	}
	if len(d.unevaluated) >= maxUnevaluatedWarnings {
		d.seenOther[snippet] = true // still dedup, so the extra count stays accurate
		d.unevaluatedExtra++
		return
	}
	d.seenOther[snippet] = true
	d.unevaluated = append(d.unevaluated, snippet)
}

// truncateSnippet shortens s to at most maxSnippetLen bytes for display,
// marking the cut with "…". It cuts on a byte boundary, which can split a
// multi-byte rune; that is acceptable for a diagnostic message about shell
// syntax, which is overwhelmingly ASCII.
func truncateSnippet(s string) string {
	if len(s) <= maxSnippetLen {
		return s
	}
	return s[:maxSnippetLen] + "…"
}

// scanDollar handles a '$' found in unquoted or double-quoted text
// (src[i] == '$'), appending either a variable segment (a bare $NAME or
// ${NAME}) or a literal '$' to word. It returns the index to resume
// scanning from, always > i.
func scanDollar(src []byte, i int, word *wordBuilder, d *dollarScan) int {
	n := len(src)
	if i+1 < n && isNameStart(src[i+1]) {
		j := i + 2
		for j < n && isNameChar(src[j]) {
			j++
		}
		name := string(src[i+1 : j])
		word.appendVar(name)
		d.addName(name)
		return j
	}
	if i+1 < n && src[i+1] == '{' {
		if end := indexByteBounded(src[i+2:], '}', maxDollarScanAhead); end >= 0 {
			inner := src[i+2 : i+2+end]
			if isSimpleName(inner) {
				name := string(inner)
				word.appendVar(name)
				d.addName(name)
				return i + 2 + end + 1
			}
			d.addUnevaluated(string(src[i : i+2+end+1]))
			word.appendByte('$')
			return i + 1
		}
	}
	// dollarSnippet's "$" fallback (nothing recognizable follows: end of
	// input, a space, punctuation, ...) is never worth its own warning —
	// it names no expansion the reader could act on, and it is exactly
	// what a stray "$" from a copy-pasted shell prompt looks like (M2,
	// phase 8 review; the prompt word itself is normally already stripped
	// by stripPromptLines before this ever runs, but a lone "$" elsewhere
	// in the input hits the same fallback and deserves the same silence).
	if snippet := dollarSnippet(src[i:]); snippet != "$" {
		d.addUnevaluated(snippet)
	}
	word.appendByte('$')
	return i + 1
}

// scanBacktick handles a backtick found in unquoted or double-quoted
// text (old-style command substitution): the whole `...` run (or, for an
// unterminated one, just the lone backtick) is left as literal text and
// reported once as an unevaluated snippet.
func scanBacktick(src []byte, i int, word *wordBuilder, d *dollarScan) int {
	if end := indexByte(src[i+1:], '`'); end >= 0 {
		snippet := src[i : i+1+end+1]
		d.addUnevaluated(string(snippet))
		word.appendBytes(snippet)
		return i + 1 + end + 1
	}
	word.appendByte('`')
	return i + 1
}

// dollarSnippet extracts a bounded, readable piece of an unrecognized '$'
// expansion for a warning message: a $(...) command substitution (no
// nested-paren tracking, and bounded to maxDollarScanAhead — see its doc
// comment and C1 — so it stops at the first ')' within that window), a
// positional parameter ($12), or a single-character special parameter
// ($?, $@, $#, $*, $!, $$, $-). Anything else (a lone '$', or one right at
// EOF) is just "$".
func dollarSnippet(s []byte) string {
	if len(s) < 2 {
		return "$"
	}
	switch {
	case s[1] == '(':
		if end := indexByteBounded(s[2:], ')', maxDollarScanAhead); end >= 0 {
			return string(s[:2+end+1])
		}
	case s[1] >= '0' && s[1] <= '9':
		j := 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		return string(s[:j])
	case strings.IndexByte("?@#*!$-", s[1]) >= 0:
		return string(s[:2])
	}
	return "$"
}

// indexByte is bytes.IndexByte without importing bytes for one call.
func indexByte(s []byte, c byte) int {
	for i, b := range s {
		if b == c {
			return i
		}
	}
	return -1
}

// indexByteBounded is indexByte, but it only looks at the first limit
// bytes of s: see maxDollarScanAhead's doc comment for why an unmatched
// '$(' or '${' must never make a scan's cost depend on how far away (or
// whether) a closer appears in the rest of the input.
func indexByteBounded(s []byte, c byte, limit int) int {
	if limit < len(s) {
		s = s[:limit]
	}
	return indexByte(s, c)
}
