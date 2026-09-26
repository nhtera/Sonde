// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"reflect"
	"testing"
)

// words extracts each word's flattened text (an expansion shown as its
// {{name}} display form), dropping separators and the startOfLine bit,
// for tests that only care about the resulting text, not which parts of
// it came from an expansion.
func words(toks []token) []string {
	var w []string
	for _, t := range toks {
		if !t.isSep {
			w = append(w, t.word.text())
		}
	}
	return w
}

// argv builds a []templatedString of plain (no expansion) words, for
// tests exercising parseArgv/parseShort directly.
func argv(words ...string) []templatedString {
	out := make([]templatedString, len(words))
	for i, w := range words {
		out[i] = litString(w)
	}
	return out
}

func TestTokenizeQuoting(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"single-quote-literal", `curl 'a b\c$d'`, []string{"curl", `a b\c$d`}},
		{"double-quote-escapes", `curl "a \"b\" \$c \\d \unchanged"`, []string{"curl", `a "b" $c \d \unchanged`}},
		{"double-quote-line-continuation", "curl \"a\\\nb\"", []string{"curl", "ab"}},
		{"backslash-escape", `curl a\ b\'c`, []string{"curl", "a b'c"}},
		{"backslash-newline-continuation", "curl \\\nhttps://a.com", []string{"curl", "https://a.com"}},
		{"backslash-crlf-continuation", "curl \\\r\nhttps://a.com", []string{"curl", "https://a.com"}},
		{"comment-to-end-of-line", "curl https://a.com # trailing comment\ncurl https://b.com", []string{"curl", "https://a.com", "curl", "https://b.com"}},
		{"comment-only-line", "# just a comment\ncurl https://a.com", []string{"curl", "https://a.com"}},
		{"hash-mid-word-not-a-comment", `curl https://a.com/a#b`, []string{"curl", "https://a.com/a#b"}},
		{"ansi-c-basic", `curl $'a\tb\nc\\d\'e\"f'`, []string{"curl", "a\tb\nc\\d'e\"f"}},
		{"ansi-c-hex-and-unicode", `curl $'\x41\x42 é'`, []string{"curl", "AB é"}},
		{"ansi-c-no-expansion", `curl $'$HOME literal'`, []string{"curl", "$HOME literal"}},
		{"unterminated-single-quote", `curl 'abc`, []string{"curl", "abc"}},
		{"unterminated-double-quote", `curl "abc`, []string{"curl", "abc"}},
		{"unterminated-ansi-c", `curl $'abc`, []string{"curl", "abc"}},
		{"trailing-lone-backslash", `curl abc\`, []string{"curl", "abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toks, _, _, _ := tokenize([]byte(tc.src))
			got := words(toks)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tokenize(%q) = %#v, want %#v", tc.src, got, tc.want)
			}
		})
	}
}

func TestTokenizeSeparatorsAndLines(t *testing.T) {
	toks, _, _, _ := tokenize([]byte("curl a; curl b && curl c"))
	var kinds []string
	for _, tok := range toks {
		if tok.isSep {
			kinds = append(kinds, string(tok.sep))
			continue
		}
		kinds = append(kinds, "word:"+tok.word.text())
	}
	want := []string{"word:curl", "word:a", ";", "word:curl", "word:b", "&", "word:curl", "word:c"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("tokenize separators = %#v, want %#v", kinds, want)
	}
}

func TestTokenizeStartOfLine(t *testing.T) {
	toks, _, _, _ := tokenize([]byte("curl a \\\n  -H x\ncurl b"))
	var flags []bool
	for _, tok := range toks {
		if !tok.isSep {
			flags = append(flags, tok.startOfLine)
		}
	}
	// "curl a -H x" is one continued logical line (backslash-newline), so
	// only its first word starts a line; "curl b" is a fresh line.
	want := []bool{true, false, false, false, true, false}
	if !reflect.DeepEqual(flags, want) {
		t.Errorf("startOfLine flags = %v, want %v", flags, want)
	}
}

func TestTokenizeEmptyInput(t *testing.T) {
	if toks, _, _, _ := tokenize(nil); len(toks) != 0 {
		t.Errorf("tokenize(nil) = %#v, want empty", toks)
	}
	if toks, _, _, _ := tokenize([]byte("   \n\t\n")); len(toks) != 0 {
		t.Errorf("tokenize(blank) = %#v, want empty", toks)
	}
}

func TestCommandsGrouping(t *testing.T) {
	toks, _, _, _ := tokenize([]byte("noise before\ncurl a\ncurl -H x \\\n  b\ncurl c; not-curl d\ncurl e && curl f"))
	cmds := commands(toks)
	var got [][]string
	for _, cmd := range cmds {
		var words []string
		for _, w := range cmd.argv {
			words = append(words, w.text())
		}
		got = append(got, words)
	}
	want := [][]string{
		{"curl", "a"},
		{"curl", "-H", "x", "b"},
		{"curl", "c"},
		{"curl", "e"},
		{"curl", "f"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commands = %#v, want %#v", got, want)
	}
}

func TestCombinedShortFlags(t *testing.T) {
	p := parseArgv(argv("curl", "-sSL", "-XPOST", "https://a.com"))
	if p.method != "POST" {
		t.Errorf("method = %q, want POST", p.method)
	}
	if !p.location {
		t.Error("-sSL did not apply -L (location)")
	}
	if len(p.urls) != 1 || p.urls[0].text() != "https://a.com" {
		t.Errorf("urls = %v, want one URL", p.urls)
	}
}

func TestCombinedShortFlagAttachedValue(t *testing.T) {
	p := parseArgv(argv("curl", "-Hx:y", "https://a.com"))
	if len(p.headers) != 1 || p.headers[0].name != "x" || p.headers[0].value.text() != "y" {
		t.Errorf("headers = %v, want [{x y}]", p.headers)
	}
}

func TestLongFlagEquals(t *testing.T) {
	p := parseArgv(argv("curl", "--header=X: Y", "--max-time=5", "https://a.com"))
	if len(p.headers) != 1 || p.headers[0].name != "X" || p.headers[0].value.text() != "Y" {
		t.Errorf("headers = %v", p.headers)
	}
	if !p.haveMaxTime || p.maxTime != "5" {
		t.Errorf("maxTime = %q, haveMaxTime = %v", p.maxTime, p.haveMaxTime)
	}
}

// TestTokenizeExpansions covers $NAME/${NAME} in unquoted and
// double-quoted text, the forms that stay literal ('...', $'...', \$),
// and every "left as literal, warn" case: $(...), a backtick, a
// positional/special parameter and a ${...} with a modifier.
func TestTokenizeExpansions(t *testing.T) {
	check := func(t *testing.T, src, wantWord string, wantNames, wantOther []string) {
		t.Helper()
		toks, names, other, _ := tokenize([]byte(src))
		if len(toks) != 2 {
			t.Fatalf("tokenize(%q) = %d tokens, want 2", src, len(toks))
		}
		// word.text() is exactly how the word would display once built
		// into a syntax.Text (toText) and rendered with {{name}}
		// placeholders shown, so this also confirms the boundary between
		// literal and expansion content is where it should be.
		if got := toks[1].word.text(); got != wantWord {
			t.Errorf("tokenize(%q) word = %q, want %q", src, got, wantWord)
		}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("tokenize(%q) names = %v, want %v", src, names, wantNames)
		}
		if !reflect.DeepEqual(other, wantOther) {
			t.Errorf("tokenize(%q) unevaluated = %v, want %v", src, other, wantOther)
		}
	}

	t.Run("bare-name-unquoted", func(t *testing.T) {
		check(t, `curl $TOKEN`, "{{TOKEN}}", []string{"TOKEN"}, nil)
	})
	t.Run("bare-name-double-quoted-in-context", func(t *testing.T) {
		check(t, `curl "Bearer $TOKEN"`, "Bearer {{TOKEN}}", []string{"TOKEN"}, nil)
	})
	t.Run("braced-name", func(t *testing.T) {
		check(t, `curl "${TOKEN}!"`, "{{TOKEN}}!", []string{"TOKEN"}, nil)
	})
	t.Run("single-quoted-stays-literal", func(t *testing.T) {
		check(t, `curl '$TOKEN'`, "$TOKEN", nil, nil)
	})
	t.Run("ansi-c-stays-literal", func(t *testing.T) {
		check(t, `curl $'$TOKEN'`, "$TOKEN", nil, nil)
	})
	t.Run("escaped-dollar-stays-literal", func(t *testing.T) {
		check(t, `curl \$TOKEN`, "$TOKEN", nil, nil)
	})
	t.Run("escaped-dollar-in-double-quotes", func(t *testing.T) {
		check(t, `curl "\$TOKEN"`, "$TOKEN", nil, nil)
	})
	t.Run("command-substitution-unevaluated", func(t *testing.T) {
		check(t, "curl \"$(date)\"", "$(date)", nil, []string{"$(date)"})
	})
	t.Run("backtick-unevaluated", func(t *testing.T) {
		check(t, "curl `whoami`", "`whoami`", nil, []string{"`whoami`"})
	})
	t.Run("positional-parameter-unevaluated", func(t *testing.T) {
		check(t, `curl $1`, "$1", nil, []string{"$1"})
	})
	t.Run("special-parameter-unevaluated", func(t *testing.T) {
		check(t, `curl $?`, "$?", nil, []string{"$?"})
	})
	t.Run("braced-with-default-unevaluated", func(t *testing.T) {
		check(t, `curl ${X:-y}`, "${X:-y}", nil, []string{"${X:-y}"})
	})
	t.Run("existing-literal-braces-untouched", func(t *testing.T) {
		check(t, `curl {{already_a_var}}`, "{{already_a_var}}", nil, nil)
	})
}
