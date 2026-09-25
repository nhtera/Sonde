// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

// A parseFunc consumes input on success. On a recoverable error the caller
// may rewind and try something else; a non-recoverable error aborts parsing.
type parseFunc[T any] func(r *reader) (T, *Error)

func errAt(pos Pos, recoverable bool, kind ErrorKind, arg string) *Error {
	return &Error{Pos: pos, Kind: kind, Arg: arg, recoverable: recoverable}
}

func expecting(pos Pos, recoverable bool, what string) *Error {
	return errAt(pos, recoverable, ErrExpecting, what)
}

// asRecoverable marks e recoverable (or not). Errors are freshly created by
// each failing rule and owned by the caller, so they are updated in place.
func asRecoverable(e *Error, recoverable bool) *Error {
	if e == errEndOfString {
		c := *e // never mutate the shared sentinel
		e = &c
	}
	e.recoverable = recoverable
	return e
}

// optional runs f; a recoverable failure rewinds and reports ok=false.
func optional[T any](r *reader, f parseFunc[T]) (v T, ok bool, err *Error) {
	save := r.pos
	v, err = f(r)
	if err == nil {
		return v, true, nil
	}
	if err.recoverable {
		r.pos = save
		var zero T
		return zero, false, nil
	}
	return v, false, err
}

// recovering makes any failure of f recoverable.
func recovering[T any](f parseFunc[T]) parseFunc[T] {
	return func(r *reader) (T, *Error) {
		v, err := f(r)
		if err != nil {
			return v, asRecoverable(err, true)
		}
		return v, nil
	}
}

// committing makes any failure of f fatal.
func committing[T any](f parseFunc[T]) parseFunc[T] {
	return func(r *reader) (T, *Error) {
		v, err := f(r)
		if err != nil {
			return v, asRecoverable(err, false)
		}
		return v, nil
	}
}

// zeroOrMore applies f until it fails recoverably (rewinding that attempt)
// or the input ends.
func zeroOrMore[T any](r *reader, f parseFunc[T]) ([]T, *Error) {
	var items []T
	for {
		save := r.pos
		if r.isEOF() {
			return items, nil
		}
		v, err := f(r)
		if err != nil {
			if err.recoverable {
				r.pos = save
				return items, nil
			}
			return nil, err
		}
		items = append(items, v)
	}
}

// oneOrMore is zeroOrMore requiring a first match; failing it is fatal.
func oneOrMore[T any](r *reader, f parseFunc[T]) ([]T, *Error) {
	first, err := f(r)
	if err != nil {
		return nil, asRecoverable(err, false)
	}
	items := []T{first}
	for {
		save := r.pos
		v, err := f(r)
		if err != nil {
			if err.recoverable {
				r.pos = save
				return items, nil
			}
			return nil, err
		}
		items = append(items, v)
	}
}

// choice tries each alternative in order, rewinding after recoverable
// failures; the last alternative's result is returned as is.
func choice[T any](r *reader, fs ...parseFunc[T]) (T, *Error) {
	for i, f := range fs {
		save := r.pos
		if i == len(fs)-1 {
			return f(r)
		}
		v, err := f(r)
		if err != nil && err.recoverable {
			r.pos = save
			continue
		}
		return v, err
	}
	panic("choice: no alternatives")
}

// alt is a choice alternative that can only succeed when the input starts
// with one of prefixes (nil prefixes: always tried).
type alt[T any] struct {
	prefixes []string
	f        parseFunc[T]
}

// prefixChoice is choice over alternatives gated by their prefixes. A
// skipped alternative would have failed recoverably at the start without
// consuming input, so the outcome equals trying them all in order; when the
// last alternative is skipped, its would-be error is returned:
// expecting its first prefix.
func prefixChoice[T any](r *reader, alts []alt[T]) (T, *Error) {
	start := r.pos
	last := len(alts) - 1
	for i, a := range alts {
		if a.prefixes != nil && !r.peekAnyPrefix(a.prefixes) {
			continue
		}
		v, err := a.f(r)
		if err == nil || !err.recoverable || i == last {
			return v, err
		}
		r.pos = start
	}
	var zero T
	return zero, expecting(start, true, alts[last].prefixes[0])
}

func (r *reader) peekAnyPrefix(prefixes []string) bool {
	for _, p := range prefixes {
		if r.peekPrefix(p) {
			return true
		}
	}
	return false
}

// keywordChoice is choice over alternatives that each begin by matching a
// fixed keyword. Alternatives whose keyword is not a prefix of the input
// would fail recoverably without consuming anything, so they are skipped;
// the result is the same as trying them all, without the failed attempts.
func keywordChoice[T any](r *reader, keywords []string, fs []parseFunc[T]) (T, *Error) {
	start := r.pos
	last := len(fs) - 1
	for i, f := range fs {
		if !r.peekPrefix(keywords[i]) {
			continue
		}
		v, err := f(r)
		if err == nil || !err.recoverable || i == last {
			return v, err
		}
		r.pos = start
	}
	var zero T
	return zero, expecting(start, true, keywords[last])
}
