// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

func (c call) regex(v value.Value) (value.Value, error) {
	re, err := c.env.Regex(c.f.Arg, c.f.Span)
	if err != nil {
		return nil, err
	}
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	if g, ok := FirstGroup(re.Re, string(s)); ok {
		return value.String(g), nil
	}
	return nil, nil
}

// FirstGroup returns the text of the first capture group of the leftmost
// match; ok is false when nothing matches or the group did not participate.
func FirstGroup(re *regexp.Regexp, s string) (string, bool) {
	m := re.FindStringSubmatchIndex(s)
	if len(m) < 4 || m[2] < 0 {
		return "", false
	}
	return s[m[2]:m[3]], true
}

func (c call) replace(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	old, err := c.arg()
	if err != nil {
		return nil, err
	}
	repl, err := c.arg2()
	if err != nil {
		return nil, err
	}
	return value.String(strings.ReplaceAll(string(s), old, repl)), nil
}

func (c call) replaceRegex(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	re, err := c.env.Regex(c.f.Arg, c.f.Span)
	if err != nil {
		return nil, err
	}
	repl, err := c.arg2()
	if err != nil {
		return nil, err
	}
	return value.String(re.Re.ReplaceAllString(string(s), repl)), nil
}

func (c call) split(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	sep, err := c.arg()
	if err != nil {
		return nil, err
	}
	var parts []string
	if sep == "" {
		// An empty separator matches before every character and at the end.
		parts = append(parts, "")
		for str := string(s); str != ""; {
			_, size := utf8.DecodeRuneInString(str)
			parts = append(parts, str[:size])
			str = str[size:]
		}
		parts = append(parts, "")
	} else {
		parts = strings.Split(string(s), sep)
	}
	list := make(value.List, len(parts))
	for i, p := range parts {
		list[i] = value.String(p)
	}
	return list, nil
}

func (c call) urlQueryParam(v value.Value) (value.Value, error) {
	name, err := c.arg()
	if err != nil {
		return nil, err
	}
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	query, reason := parseURLQuery(string(s))
	if reason != "" {
		e := runerr.New(c.f.Span, runerr.InvalidURL, c.assert)
		e.Value, e.Reason = string(s), reason
		return nil, e
	}
	for pair := range strings.SplitSeq(query, "&") {
		if pair == "" {
			continue
		}
		k, val, _ := strings.Cut(pair, "=")
		if percentDecode(k, true) == name {
			return value.String(percentDecode(val, true)), nil
		}
	}
	return nil, nil
}

// parseURLQuery checks that s is an absolute http(s) URL and returns its
// raw query; reason explains an invalid URL.
func parseURLQuery(s string) (query, reason string) {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		if scheme, _, ok := strings.Cut(s, "://"); ok && scheme != "" && strings.Trim(scheme, "abcdefghijklmnopqrstuvwxyz") == "" {
			return "", "Only <http://> and <https://> schemes are supported"
		}
		return "", "Missing scheme <http://> or <https://>"
	}
	u, err := url.Parse(s)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return "", ue.Err.Error()
		}
		return "", err.Error()
	}
	if u.Host == "" {
		return "", "empty host"
	}
	return u.RawQuery, ""
}
