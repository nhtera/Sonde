// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import "unicode/utf8"

// This file implements a small, self-contained shell-word tokenizer. It
// exists because the generated exporter (engine/curl.go) emits $'...'
// ANSI-C quoting, which mattn/go-shellwords (and most lightweight shell
// lexers) do not read back, and this package must round-trip its own
// output. It understands only what a curl one-liner actually needs:
// single/double quotes, $'...' quoting, backslash escapes, line
// continuations, '#' comments and $NAME/${NAME} parameter expansions
// (expand.go). It never panics on malformed input; an unterminated quote
// simply runs to the end of the input.

// token is one lexical unit produced by tokenize: a word, or a command
// separator (';' or '&' for "&&").
type token struct {
	word  templatedString
	isSep bool
	sep   byte // ';' or '&' ("&&"); meaningful only when isSep
	// startOfLine is true when word is the first word of a logical source
	// line (the very first word of the input, or the first word after an
	// unescaped newline); a backslash-newline continuation does not start
	// a new line. Meaningful only when !isSep.
	startOfLine bool
	// start and end are word's raw byte offsets into tokenize's src (end
	// exclusive), used only to correlate a command's span back to the raw
	// input — see command.go's windowsCmdSpans and the M2/decision-3
	// Windows cmd ^ detection. Meaningless when isSep.
	start, end int
}

// tokenize splits src into words and ';'/'&&' separators, and reports
// every distinct shell parameter expansion it found: names (a bare
// $NAME or ${NAME}, each mapped to a variable segment of the matching
// word — see expand.go) and unevaluated (any other '$'/backtick shell
// substitution, left as literal text since this package never runs a
// shell), each in first-seen order.
//
// Quoting and escaping: '...' is literal with no escapes and no
// expansions; "..." recognizes backslash before $, `, ", \ and newline (a
// line continuation), leaves any other backslash sequence untouched
// matching a POSIX shell, and expands $NAME/${NAME}; $'...' applies
// ANSI-C escapes (see decodeANSIC) and never expands. A bare backslash
// outside quotes escapes the next character, or, before a newline (LF or
// CRLF), joins the two lines; unquoted text also expands $NAME/${NAME}.
// '#' starts a comment to the end of the physical line, but only where a
// new word may start (not inside or right after another word). ';', an
// unquoted "&&", and every shell pipe/redirect operator this package
// recognizes (a lone '&', '|', '<', '>'/'>>', with or without a leading
// bare file-descriptor number like the "2" of "2>&1") end the current
// word and are reported as separators (M2, phase 8 review): what follows
// one is never more curl arguments, so commands() simply stops collecting
// into the current command at that point.
func tokenize(src []byte) (toks []token, names []string, unevaluated []string, unevaluatedExtra int) {
	n := len(src)
	i := 0
	atLineStart := true
	var word wordBuilder
	haveWord := false
	wordStartOfLine := false
	wordStart := 0
	d := newDollarScan()

	flush := func() {
		if haveWord {
			toks = append(toks, token{word: word.build(), startOfLine: wordStartOfLine, start: wordStart, end: i})
			haveWord = false
		}
	}
	startWord := func() {
		if !haveWord {
			wordStartOfLine = atLineStart
			atLineStart = false
			haveWord = true
			wordStart = i
		}
	}
	newline := func() {
		flush()
		atLineStart = true
	}

	for i < n {
		c := src[i]
		switch {
		case c == '\\' && i+1 < n && src[i+1] == '\n':
			i += 2
		case c == '\\' && i+2 < n && src[i+1] == '\r' && src[i+2] == '\n':
			i += 3
		case c == '\r' && i+1 < n && src[i+1] == '\n':
			newline()
			i += 2
		case c == '\n' || c == '\r':
			newline()
			i++
		case c == ' ' || c == '\t':
			flush()
			i++
		case c == '#' && !haveWord:
			for i < n && src[i] != '\n' && src[i] != '\r' {
				i++
			}
		case c == ';':
			flush()
			toks = append(toks, token{isSep: true, sep: ';'})
			i++
		case c == '&' && i+1 < n && src[i+1] == '&':
			flush()
			toks = append(toks, token{isSep: true, sep: '&'})
			i += 2
		case c == '|' || c == '<' || c == '&':
			// A pipe, input redirect or lone '&' (background/fd-dup, once
			// "&&" above didn't match): whatever follows belongs to a
			// different command (or isn't a curl argument at all), not
			// this one — M2, phase 8 review. Reported as a separator, the
			// same as ';', so commands() simply stops collecting into the
			// current command; anything before the next real "curl" word
			// is then dropped by its own existing "not curl: dropped"
			// rule, with no special-case needed here for what follows.
			flush()
			toks = append(toks, token{isSep: true, sep: c})
			i++
		case c == '>':
			// A ">"/">>" output redirect, optionally preceded by a bare
			// file-descriptor number with no space ("2>", "2>&1", not
			// "2 >", which is not valid shell syntax anyway): that number
			// was already accumulated as an ordinary in-progress word, so
			// discard it here rather than flushing it as a stray one.
			if haveWord && word.isDigitsOnly() {
				word = wordBuilder{}
				haveWord = false
			} else {
				flush()
			}
			toks = append(toks, token{isSep: true, sep: '>'})
			i++
			if i < n && src[i] == '>' {
				i++ // ">>" (append redirect): still just one terminator
			}
		case c == '\'':
			startWord()
			i++
			for i < n && src[i] != '\'' {
				word.appendByte(src[i])
				i++
			}
			if i < n {
				i++
			}
		case c == '"':
			startWord()
			i++
			i = scanDoubleQuoted(src, i, &word, d)
		case c == '$' && i+1 < n && src[i+1] == '\'':
			startWord()
			i += 2
			i = scanANSIC(src, i, &word)
		case c == '$':
			startWord()
			i = scanDollar(src, i, &word, d)
		case c == '`':
			startWord()
			i = scanBacktick(src, i, &word, d)
		case c == '\\':
			startWord()
			if i+1 < n {
				word.appendByte(src[i+1])
				i += 2
			} else {
				i++ // a trailing lone backslash escapes nothing
			}
		default:
			startWord()
			word.appendByte(c)
			i++
		}
	}
	flush()
	return toks, d.names, d.unevaluated, d.unevaluatedExtra
}

// scanDoubleQuoted appends the content of a "..." string starting at i
// (just past the opening quote) to word, and returns the index just past
// the closing quote (or n, for an unterminated string).
func scanDoubleQuoted(src []byte, i int, word *wordBuilder, d *dollarScan) int {
	n := len(src)
	for i < n && src[i] != '"' {
		switch {
		case src[i] == '$':
			i = scanDollar(src, i, word, d)
		case src[i] == '`':
			i = scanBacktick(src, i, word, d)
		case src[i] != '\\' || i+1 >= n:
			word.appendByte(src[i])
			i++
		default:
			switch nc := src[i+1]; {
			case nc == '$' || nc == '`' || nc == '"' || nc == '\\':
				word.appendByte(nc)
				i += 2
			case nc == '\n':
				i += 2 // line continuation
			case nc == '\r' && i+2 < n && src[i+2] == '\n':
				i += 3
			default:
				word.appendByte('\\')
				word.appendByte(nc)
				i += 2
			}
		}
	}
	if i < n {
		i++
	}
	return i
}

// scanANSIC appends the decoded content of a $'...' string starting at i
// (just past the opening quote) to word, and returns the index just past
// the closing quote (or n, for an unterminated string). $'...' never
// expands a parameter: bash itself treats it as a plain quoted string
// once the backslash escapes are resolved.
func scanANSIC(src []byte, i int, word *wordBuilder) int {
	n := len(src)
	for i < n && src[i] != '\'' {
		if src[i] != '\\' {
			word.appendByte(src[i])
			i++
			continue
		}
		consumed, decoded := decodeANSIC(src[i:])
		word.appendBytes(decoded)
		i += consumed
	}
	if i < n {
		i++
	}
	return i
}

// decodeANSIC decodes one backslash escape of a $'...' string (s[0] ==
// '\\'), returning how many input bytes it consumed (always >= 1) and its
// decoded bytes. An escape this package does not recognize is passed
// through unchanged (backslash and the following byte).
func decodeANSIC(s []byte) (consumed int, decoded []byte) {
	if len(s) < 2 {
		return 1, []byte{'\\'}
	}
	switch s[1] {
	case '\\':
		return 2, []byte{'\\'}
	case '\'':
		return 2, []byte{'\''}
	case '"':
		return 2, []byte{'"'}
	case '?':
		return 2, []byte{'?'}
	case 'a':
		return 2, []byte{'\a'}
	case 'b':
		return 2, []byte{'\b'}
	case 'e', 'E':
		return 2, []byte{0x1b}
	case 'f':
		return 2, []byte{'\f'}
	case 'n':
		return 2, []byte{'\n'}
	case 'r':
		return 2, []byte{'\r'}
	case 't':
		return 2, []byte{'\t'}
	case 'v':
		return 2, []byte{'\v'}
	case 'x':
		if digits, val, ok := readHex(s[2:], 2); ok {
			return 2 + digits, []byte{byte(val & 0xFF)} // at most 2 hex digits: val <= 0xFF
		}
		return 2, []byte{'x'}
	case 'u':
		if digits, val, ok := readHex(s[2:], 4); ok {
			return 2 + digits, utf8.AppendRune(nil, runeFromCodePoint(val))
		}
		return 2, []byte{'u'}
	case 'U':
		if digits, val, ok := readHex(s[2:], 8); ok {
			return 2 + digits, utf8.AppendRune(nil, runeFromCodePoint(val))
		}
		return 2, []byte{'U'}
	case '0', '1', '2', '3', '4', '5', '6', '7':
		digits, val := readOctal(s[1:], 3)
		return 1 + digits, []byte{byte(val & 0xFF)} // bash masks an octal escape to one byte
	default:
		return 2, []byte{'\\', s[1]}
	}
}

// runeFromCodePoint converts a \u/\U escape's decoded value to a rune,
// substituting utf8.RuneError for one outside the valid Unicode range (a
// \U escape can read up to 8 hex digits, far more than any valid code
// point), so the conversion can never wrap into an unrelated rune.
func runeFromCodePoint(val int) rune {
	if val < 0 || val > utf8.MaxRune {
		return utf8.RuneError
	}
	return rune(val)
}

// readHex reads up to maxDigits hex digits from the start of s, returning
// how many it read and their value; ok is false when s starts with no hex
// digit at all.
func readHex(s []byte, maxDigits int) (digits int, val int, ok bool) {
	for digits < maxDigits && digits < len(s) {
		d, isHex := hexDigit(s[digits])
		if !isHex {
			break
		}
		val = val*16 + d
		digits++
	}
	return digits, val, digits > 0
}

// readOctal reads up to maxDigits octal digits from the start of s.
func readOctal(s []byte, maxDigits int) (digits int, val int) {
	for digits < maxDigits && digits < len(s) && s[digits] >= '0' && s[digits] <= '7' {
		val = val*8 + int(s[digits]-'0')
		digits++
	}
	return digits, val
}

func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}
