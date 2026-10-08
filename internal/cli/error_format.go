// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// maxLongFormatBytes is the number of leading body bytes shown for a
// binary response under --error-format long, matching the upstream
// CLI's own preview size.
const maxLongFormatBytes = 64

// writeLongFormatErrors prints res's errors in the "--error-format long"
// shape: for each entry attempt that decided the outcome (Retried is
// false; a retried attempt is skipped, matching the engine's own
// short-form suppression during a retry) and has errors, the curl command
// of its request, its last response's status/headers/body, then the same
// "error: ..." block the short form prints. It replaces the engine's own
// LogError events for the run (see eventLogger.longErrors), so every
// error still gets rendered exactly once.
func writeLongFormatErrors(stderr io.Writer, res *engine.UnitResult, color bool) {
	redact := res.Redact
	hasSecrets := res.HasSecrets()
	for i, e := range res.Entries {
		if !res.Decisive(i) {
			continue // a retried attempt, or a repeat a later one replaced
		}
		if len(e.Errors) == 0 {
			continue
		}
		if len(e.Calls) > 0 {
			call := e.Calls[len(e.Calls)-1]
			if e.Curl != "" { // none for a WebSocket or gRPC entry
				writeCurlHint(stderr, redact(e.Curl), color)
				fmt.Fprintln(stderr)
			}
			if call.Response != nil {
				writeStatusAndHeaders(stderr, call.Response, color, redact)
				writeLongFormatBody(stderr, call.Response, redact, hasSecrets)
				fmt.Fprintln(stderr)
			}
		}
		for _, err := range e.Errors {
			rendered := err.Render()
			if color {
				rendered = err.RenderColor()
			}
			writePrefixedError(stderr, redact(strings.ReplaceAll(rendered, "\r\n", "\n")), color)
		}
	}
}

// writeCurlHint writes the two "* Request can be run with the following
// curl command:\n* <cmd>" lines, colored like any other "* " debug line.
func writeCurlHint(w io.Writer, cmd string, color bool) {
	if !color {
		fmt.Fprintf(w, "* Request can be run with the following curl command:\n* %s\n", cmd) //nolint:errcheck // stderr write
		return
	}
	fmt.Fprintf(w, "%s*%s Request can be run with the following curl command:\n%s*%s %s\n",
		ansiBlueBold, ansiReset, ansiBlueBold, ansiReset, cmd) //nolint:errcheck
}

// writePrefixedError writes "error: <rendered>\n\n"; a colored rendered
// error carries its own colors.
func writePrefixedError(w io.Writer, rendered string, color bool) {
	if !color {
		fmt.Fprintf(w, "error: %s\n\n", rendered) //nolint:errcheck
		return
	}
	fmt.Fprintf(w, "%serror%s: %s\n\n", ansiRedBold, ansiReset, rendered) //nolint:errcheck
}

// writeLongFormatBody writes r's body as text when its content type is
// kind of text (decoding any content-encoding first, then redacting the
// decoded text through redact), else as a hex-encoded preview of up to
// maxLongFormatBytes raw bytes. skipBinaryPreview skips that preview
// entirely (the run has secrets registered, and a hex dump of raw bytes
// cannot be redacted the way text can — a registered secret may or may
// not appear in it byte for byte, so omitting it is the only safe
// choice).
func writeLongFormatBody(w io.Writer, r *exchange.Response, redact func(string) string, skipBinaryPreview bool) {
	if ct, ok := r.ContentType(); ok && !isKindOfText(ct) {
		if !skipBinaryPreview {
			writeBytesPreview(w, r.Body)
		}
		return
	}
	text, err := r.Text()
	if err != nil {
		if !skipBinaryPreview {
			writeBytesPreview(w, r.Body)
		}
		return
	}
	fmt.Fprintln(w, redact(text)) //nolint:errcheck
}

func writeBytesPreview(w io.Writer, body []byte) {
	if len(body) == 0 {
		fmt.Fprintln(w) //nolint:errcheck
		return
	}
	if len(body) > maxLongFormatBytes {
		body = body[:maxLongFormatBytes]
	}
	fmt.Fprintf(w, "Bytes <%s...>\n", hex.EncodeToString(body)) //nolint:errcheck
}

// isKindOfText reports whether contentType can be decoded and shown as
// text: any "text/*" type, JSON (including a "+json"/"-json"/".json"
// structured syntax suffix, e.g. "application/problem+json"), or XML
// (including a "+xml"/".xml" suffix).
func isKindOfText(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(ct, "text/"):
		return true
	case strings.HasPrefix(ct, "application/json"):
		return true
	case strings.HasPrefix(ct, "application/xml"):
		return true
	}
	base, _, _ := strings.Cut(ct, ";")
	if !strings.HasPrefix(base, "application/") {
		return false
	}
	for _, suffix := range []string{"+json", ".json", "-json", "+xml", ".xml"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}
