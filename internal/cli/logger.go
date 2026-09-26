// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nhtera/sonde/engine"
)

// ANSI styles used for log lines, applied only when color is on.
const (
	ansiReset      = "\x1b[0m"
	ansiBold       = "\x1b[1m"
	ansiBlueBold   = "\x1b[1;34m" // "* " prefix
	ansiPurpleBold = "\x1b[1;35m" // request line
	ansiGreenBold  = "\x1b[1;32m" // status line
	ansiCyanBold   = "\x1b[1;36m" // request/response header name
	ansiYellowBold = "\x1b[1;33m" // capture name, warning label
	ansiRedBold    = "\x1b[1;31m" // error label
)

// eventLogger renders engine.Log events to stderr in the documented
// format: "* " for debug, "> "/"< " for the request/response as sent and
// received, "warning: "/"error: " for problems, with a blank line after
// an error.
type eventLogger struct {
	stderr io.Writer
	color  bool
	// longErrors is true for --error-format long: the engine's own
	// LogError events are suppressed (writeLongFormatErrors in
	// error_format.go prints the richer form itself, once per finished
	// entry, instead).
	longErrors bool
}

func newEventLogger(stderr io.Writer, color, longErrors bool) *eventLogger {
	return &eventLogger{stderr: stderr, color: color, longErrors: longErrors}
}

// handle is passed as engine.Options.OnEvent.
func (l *eventLogger) handle(ev engine.Event) {
	log, ok := ev.(engine.Log)
	if !ok {
		return
	}
	l.writeLog(log)
}

func (l *eventLogger) writeLog(log engine.Log) {
	switch log.Level {
	case engine.LogDebug:
		l.writeStar(log.Text, false)
	case engine.LogDebugImportant:
		l.writeStar(log.Text, true)
	case engine.LogCapture:
		l.writeCapture(log.Text)
	case engine.LogDebugError:
		for line := range strings.SplitSeq(l.errorText(log), "\n") {
			l.writeStar(line, false)
		}
	case engine.LogRequestLine:
		l.writeDirectionLine(">", ansiPurpleBold, log.Text)
	case engine.LogRequest:
		l.writeDirectionHeader(">", log.Text)
	case engine.LogResponseLine:
		l.writeDirectionLine("<", ansiGreenBold, log.Text)
	case engine.LogResponse:
		l.writeDirectionHeader("<", log.Text)
	case engine.LogWarning:
		l.writePrefixedMessage(ansiYellowBold, "warning", log.Text)
	case engine.LogError:
		if !l.longErrors {
			l.writeError(l.errorText(log))
		}
	}
}

// errorText is the rendered error of a log event, colored if need be.
func (l *eventLogger) errorText(log engine.Log) string {
	if l.color && log.Color != "" {
		return log.Color
	}
	return log.Text
}

// writeError writes "error: <rendered>" and a blank line; the rendered
// error carries its own colors.
func (l *eventLogger) writeError(rendered string) {
	writePrefixedError(l.stderr, rendered, l.color)
}

// writeStar writes "* <text>", or a bare "*" when text is empty (a blank
// debug separator line); bold additionally emphasizes the text, as
// LogDebugImportant does.
func (l *eventLogger) writeStar(text string, bold bool) {
	if !l.color {
		if text == "" {
			l.writePlain("*")
			return
		}
		l.writePlain("* " + text)
		return
	}
	if text == "" {
		fmt.Fprintf(l.stderr, "%s*%s\n", ansiBlueBold, ansiReset) //nolint:errcheck // stderr write, nothing to do on failure
		return
	}
	if bold {
		fmt.Fprintf(l.stderr, "%s*%s %s%s%s\n", ansiBlueBold, ansiReset, ansiBold, text, ansiReset) //nolint:errcheck
		return
	}
	fmt.Fprintf(l.stderr, "%s*%s %s\n", ansiBlueBold, ansiReset, text) //nolint:errcheck
}

// writeCapture writes a LogCapture line ("* name: value"), coloring only
// the capture's name.
func (l *eventLogger) writeCapture(text string) {
	if !l.color {
		l.writePlain("* " + text)
		return
	}
	name, rest, ok := strings.Cut(text, ": ")
	if !ok {
		fmt.Fprintf(l.stderr, "%s*%s %s\n", ansiBlueBold, ansiReset, text) //nolint:errcheck
		return
	}
	fmt.Fprintf(l.stderr, "%s*%s %s%s%s: %s\n", ansiBlueBold, ansiReset, ansiYellowBold, name, ansiReset, rest) //nolint:errcheck
}

// writeDirectionLine writes "<prefix> <text>" (a request or status line),
// coloring the whole text in style.
func (l *eventLogger) writeDirectionLine(prefix, style, text string) {
	if !l.color {
		l.writePlain(prefix + " " + text)
		return
	}
	fmt.Fprintf(l.stderr, "%s %s%s%s\n", prefix, style, text, ansiReset) //nolint:errcheck
}

// writeDirectionHeader writes "<prefix> Name: value" (a request or
// response header), coloring only the header name; an empty text is the
// terminator after the last header, printed as a bare prefix.
func (l *eventLogger) writeDirectionHeader(prefix, text string) {
	if text == "" {
		l.writePlain(prefix)
		return
	}
	if !l.color {
		l.writePlain(prefix + " " + text)
		return
	}
	name, rest, ok := strings.Cut(text, ": ")
	if !ok {
		l.writePlain(prefix + " " + text)
		return
	}
	fmt.Fprintf(l.stderr, "%s %s%s%s: %s\n", prefix, ansiCyanBold, name, ansiReset, rest) //nolint:errcheck
}

// writePrefixedMessage writes "<prefix>: <message>" ("warning: ...",
// "error: ..."); an empty message prints nothing.
func (l *eventLogger) writePrefixedMessage(style, prefix, text string) {
	if text == "" {
		return
	}
	if !l.color {
		fmt.Fprintf(l.stderr, "%s: %s\n", prefix, text) //nolint:errcheck
		return
	}
	fmt.Fprintf(l.stderr, "%s%s%s: %s%s%s\n", style, prefix, ansiReset, ansiBold, text, ansiReset) //nolint:errcheck
}

func (l *eventLogger) writePlain(text string) {
	fmt.Fprintln(l.stderr, text) //nolint:errcheck // stderr write, nothing to do on failure
}
