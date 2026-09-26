// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import "sort"

// command is one grouped curl invocation: its argv, plus enough of its raw
// source span to check for a Windows cmd.exe marker (see windowsCmd).
type command struct {
	argv []templatedString
	// start and end are this command's raw byte span in tokenize's src
	// (end exclusive): the first word's start through the last word's end.
	start, end int
}

// commands groups tokenize's output into argv lists, one per curl
// invocation: a new command starts at a "curl" word at the start of a
// logical line, or right after a ';' or "&&" separator (again requiring a
// "curl" word). Words that appear before the first such start, or right
// after a separator that is not followed by "curl", are dropped: they are
// either shell noise (e.g. an unrelated command joined with ';') or,
// without any prior "curl" word at all, not a curl script.
func commands(toks []token) []command {
	var cmds []command
	var current command
	open := false

	finish := func() {
		if open {
			cmds = append(cmds, current)
			current = command{}
			open = false
		}
	}
	isCurl := func(w templatedString) bool { return w.text() == "curl" }
	start := func(t token) {
		current = command{argv: []templatedString{t.word}, start: t.start, end: t.end}
		open = true
	}

	for _, t := range toks {
		if t.isSep {
			finish()
			continue
		}
		switch {
		case !open && isCurl(t.word):
			start(t)
		case open && t.startOfLine && isCurl(t.word):
			finish()
			start(t)
		case open:
			current.argv = append(current.argv, t.word)
			current.end = t.end
			// !open && word != "curl": dropped.
		}
	}
	finish()
	return cmds
}

// windowsCmdReason is the Skipped reason for a command whose source uses
// cmd.exe (Windows Command Prompt) syntax rather than a POSIX shell's: a
// "^" line continuation (cmd's equivalent of bash's trailing "\") or a
// "^\"" escaped quote. Neither means anything special to this tokenizer
// (bash has no such escape), so without this check a command copied from
// "Copy as cURL (cmd)" would silently import with a stray caret glued
// onto a header or URL value instead of being rejected outright.
const windowsCmdReason = "Windows cmd syntax is not supported; copy the command as cURL (bash)"

// windowsCmdMarkers returns the byte offset of every "^\n"/"^\r\n" (a
// cmd.exe line continuation) and "^\"" (a cmd.exe escaped quote) in src,
// pointing at the '^' itself.
func windowsCmdMarkers(src []byte) []int {
	var marks []int
	for i := 0; i < len(src); i++ {
		if src[i] != '^' {
			continue
		}
		switch {
		case i+1 < len(src) && src[i+1] == '\n',
			i+2 < len(src) && src[i+1] == '\r' && src[i+2] == '\n',
			i+1 < len(src) && src[i+1] == '"':
			marks = append(marks, i)
		}
	}
	return marks
}

// isWindowsCmd reports whether any windowsCmdMarkers offset falls inside
// c's raw source span [start, end). marks is in ascending order (built by
// a single forward scan), so this is a binary search rather than a linear
// scan per command: with up to maxCommands commands and up to O(len(src))
// markers, a linear scan per command would reintroduce the same
// quadratic-work shape C1 fixed elsewhere in this package.
func (c command) isWindowsCmd(marks []int) bool {
	i := sort.Search(len(marks), func(i int) bool { return marks[i] >= c.start })
	return i < len(marks) && marks[i] < c.end
}
