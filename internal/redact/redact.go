// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package redact holds a per-run, append-only registry of secret values and
// masks them, and their predictable encoded variants, out of text before it
// is printed, logged, or written to a report.
//
// One Registry is created per run. Add is called whenever a secret value
// becomes known (a captured header, a --variable, a field marked to be
// redacted); it never forgets a value, so a secret captured early in a run
// stays masked even after a later capture overwrites the name that held it.
// Redact and RedactBytes replace the raw value, and for each value its
// base64 (standard and URL-safe, padded and unpadded), URL-escaped (query
// and path), and JSON-escaped (with and without HTML escaping) forms, with
// Mask. Every occurrence of every pattern is found and overlapping or
// adjacent occurrences are masked as one, so no part of a secret survives
// even when secrets overlap or contain each other.
//
// A Registry is safe for concurrent use: Add, Redact and the other methods
// may be called from multiple goroutines at once.
package redact

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Mask replaces every matched occurrence of a registered secret.
const Mask = "***"

// MinLength is the rune length below which Add reports a newly registered
// value as short, so the caller can warn that a short secret is prone to
// matching text it was never meant to redact. Short secrets are still
// redacted; the warning is the caller's responsibility.
const MinLength = 4

// Registry is a per-run, append-only store of secret values to mask out of
// text. See the package doc for what is matched and the concurrency
// guarantee. The zero value is not guaranteed usable; use New.
type Registry struct {
	mu     sync.RWMutex
	values map[string]struct{}
	index  *patternIndex // raw values and variants; nil when stale
	built  bool
}

// New returns an empty Registry ready for concurrent use.
func New() *Registry {
	return &Registry{}
}

// Add registers value under name. The empty string is never registered
// (Redact never masks ""), and re-registering a value already known to the
// registry is a no-op. Every other value, once added, stays masked for the
// life of the Registry, even after name is given a different value.
//
// short reports whether value was newly registered and is shorter than
// MinLength runes; the caller may want to warn about that, since short
// secrets are prone to matching unrelated text. value is redacted either
// way.
func (r *Registry) Add(name, value string) (short bool) { //nolint:revive // name documents the call site; matching is by value only
	if value == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[value]; ok {
		return false
	}
	if r.values == nil {
		r.values = make(map[string]struct{})
	}
	r.values[value] = struct{}{}
	r.index = nil
	r.built = false
	return utf8.RuneCountInString(value) < MinLength
}

// Values returns a sorted copy of every distinct raw secret value
// registered so far. It is meant for run-level sinks (reports, logs)
// written once the run has finished.
func (r *Registry) Values() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.values))
	for v := range r.values {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Len returns the number of distinct raw secret values registered so far.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.values)
}

// Redact returns s with every occurrence of every registered secret value,
// and its encoded variants, replaced by Mask. s is returned unchanged, and
// without allocating, if nothing matches.
func (r *Registry) Redact(s string) string {
	return redact(s, r.getIndex(), nil)
}

// RedactWith is Redact over the secrets of r and of extra together, in
// one pass, so that overlapping secrets of the two registries are masked
// as one. extra may be nil.
func (r *Registry) RedactWith(s string, extra *Registry) string {
	if extra == nil {
		return r.Redact(s)
	}
	return redact(s, r.getIndex(), extra.getIndex())
}

// redact masks the patterns of a and b (either may be nil) in s.
func redact(s string, a, b *patternIndex) string {
	if a == nil && b == nil {
		return s
	}
	var out strings.Builder
	last := 0            // start of the text not yet copied
	start, end := -1, -1 // current merged match
	for i := 0; i < len(s); i++ {
		n := max(longestMatch(s[i:], a), longestMatch(s[i:], b))
		if n == 0 {
			continue
		}
		switch {
		case start >= 0 && i <= end:
			end = max(end, i+n)
		default:
			if start >= 0 {
				out.WriteString(s[last:start])
				out.WriteString(Mask)
				last = end
			}
			start, end = i, i+n
		}
	}
	if start < 0 {
		return s
	}
	out.WriteString(s[last:start])
	out.WriteString(Mask)
	out.WriteString(s[end:])
	return out.String()
}

// longestMatch returns the length of the longest pattern of idx that s
// starts with, or 0.
func longestMatch(s string, idx *patternIndex) int {
	if idx == nil || s == "" {
		return 0
	}
	for _, p := range idx[s[0]] {
		if strings.HasPrefix(s, p) {
			return len(p) // patterns are longest first
		}
	}
	return 0
}

// RedactBytes is Redact for a byte slice.
func (r *Registry) RedactBytes(b []byte) []byte {
	if r.getIndex() == nil {
		return b
	}
	return []byte(r.Redact(string(b)))
}

// patternIndex lists the patterns by first byte, longest first.
type patternIndex [256][]string

// getIndex returns the cached pattern index, rebuilding it if the last Add
// invalidated it; nil when no secret is registered.
func (r *Registry) getIndex() *patternIndex {
	r.mu.RLock()
	if r.built {
		idx := r.index
		r.mu.RUnlock()
		return idx
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.built {
		r.index = buildIndex(r.values)
		r.built = true
	}
	return r.index
}

// buildIndex indexes every value and its encoded variants by first byte,
// deduplicated and sorted longest first (then lexicographically).
func buildIndex(values map[string]struct{}) *patternIndex {
	if len(values) == 0 {
		return nil
	}
	patternSet := make(map[string]struct{}, len(values))
	for v := range values {
		patternSet[v] = struct{}{}
		for _, variant := range variants(v) {
			patternSet[variant] = struct{}{}
		}
	}
	patterns := make([]string, 0, len(patternSet))
	for p := range patternSet {
		patterns = append(patterns, p)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})
	idx := new(patternIndex)
	for _, p := range patterns {
		idx[p[0]] = append(idx[p[0]], p)
	}
	return idx
}
