// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import "strings"

// Unicode definitions of the Perl classes. Go's RE2 classes are ASCII-only;
// test files expect \d, \w and \s to match any Unicode digit, word character
// and white space.
const (
	digitClass = `\p{Nd}`
	wordClass  = `\p{L}\p{Nl}\p{M}\p{Nd}\p{Pc}\x{200C}\x{200D}`
	spaceClass = `\t\n\v\f\r \x{85}\x{A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}`
)

// TranslateRegex rewrites \d, \D, \w, \W, \s and \S to their Unicode
// definitions. Inside a bracket class \W and \S cannot be expressed as a
// union and keep their ASCII meaning; \b stays ASCII as well.
func TranslateRegex(pattern string) string {
	if !strings.Contains(pattern, `\`) {
		return pattern
	}
	var b strings.Builder
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\\' && i+1 < len(pattern):
			n := pattern[i+1]
			i++
			if s, ok := perlClass(n, inClass); ok {
				b.WriteString(s)
				continue
			}
			b.WriteByte(c)
			b.WriteByte(n)
			if (n == 'p' || n == 'P' || n == 'x') && i+1 < len(pattern) && pattern[i+1] == '{' {
				end := strings.IndexByte(pattern[i+1:], '}')
				if end >= 0 {
					b.WriteString(pattern[i+1 : i+2+end])
					i += end + 1
				}
			}
		case c == '[' && !inClass:
			inClass = true
			b.WriteByte(c)
			// A ']' right after '[' or '[^' is a literal.
			if i+1 < len(pattern) && pattern[i+1] == '^' {
				b.WriteByte('^')
				i++
			}
			if i+1 < len(pattern) && pattern[i+1] == ']' {
				b.WriteByte(']')
				i++
			}
		case c == '[' && inClass && i+1 < len(pattern) && pattern[i+1] == ':':
			end := strings.Index(pattern[i:], ":]")
			if end < 0 {
				b.WriteByte(c)
				continue
			}
			b.WriteString(pattern[i : i+end+2])
			i += end + 1
		case c == ']' && inClass:
			inClass = false
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func perlClass(c byte, inClass bool) (string, bool) {
	switch c {
	case 'd':
		return digitClass, true
	case 'D':
		return `\P{Nd}`, true
	case 'w':
		if inClass {
			return wordClass, true
		}
		return "[" + wordClass + "]", true
	case 's':
		if inClass {
			return spaceClass, true
		}
		return "[" + spaceClass + "]", true
	case 'W':
		if !inClass {
			return "[^" + wordClass + "]", true
		}
	case 'S':
		if !inClass {
			return "[^" + spaceClass + "]", true
		}
	}
	return "", false
}
