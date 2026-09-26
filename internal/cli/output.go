// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
)

// outputSink is where a run's file output goes: stdout, or a -o/--output
// file opened lazily on first use so a write failure can be attributed to
// the entry that triggered it. Once open, the same file is reused for the
// rest of the invocation (truncated once, appended to after that).
type outputSink struct {
	path   string
	stdout io.Writer
	file   *os.File
}

// newOutputSink returns a sink writing to stdout when path is "" or "-",
// else lazily to path.
func newOutputSink(path string, stdout io.Writer) *outputSink {
	return &outputSink{path: path, stdout: stdout}
}

// writer returns the underlying writer, opening (and truncating) path on
// first use.
func (s *outputSink) writer() (io.Writer, error) {
	if s.path == "" || s.path == "-" {
		return s.stdout, nil
	}
	if s.file == nil {
		f, err := os.Create(s.path) //nolint:gosec // G304: the command line names the file
		if err != nil {
			return nil, err
		}
		s.file = f
	}
	return s.file, nil
}

// Close releases the sink's file, if one was opened. Always safe to call.
func (s *outputSink) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// outputError is a CLI-triggered runtime error — an -o/--output write
// failure, or a --compressed decode failure — rendered the same way the
// engine renders an entry error. The run loop prints it to stderr and
// folds it into the run's worst exit code (ExitRuntime) instead of
// aborting the whole invocation.
type outputError struct{ rendered string }

func (e *outputError) Error() string { return e.rendered }

// writeFileOutput writes what `sonde [options] FILE` prints to stdout (or
// -o FILE) for one file's result: the last response body, optionally with
// headers (-i) and pretty-printed (--pretty), or the whole run as one
// --json line. It never writes anything for a file that did not succeed,
// unless --json was given (the JSON result is always emitted, success or
// not).
func writeFileOutput(rc *runContext, sink *outputSink, runner *engine.Runner, res *engine.UnitResult) error {
	if rc.jsonOutput {
		w, err := sink.writer()
		if err != nil {
			return fileWriteError(res, outputName(rc.output), err)
		}
		return writeJSONLine(w, runner, res)
	}
	if rc.noOutput || res.ParseError != nil || !res.Success {
		return nil
	}
	entry, call := lastEntryAndCall(res)
	if call == nil || call.Response == nil {
		return nil
	}
	w, err := sink.writer()
	if err != nil {
		return fileWriteError(res, outputName(rc.output), err)
	}
	if rc.include {
		writeStatusAndHeaders(w, call.Response, rc.color, identityString)
	}
	body := call.Response.Body
	if entry.Compressed {
		decoded, derr := call.Response.DecodedBody()
		if derr != nil {
			var be *exchange.BodyError
			if errors.As(derr, &be) {
				return decompressionError(res, be)
			}
			return derr
		}
		body = decoded
	}
	if rc.pretty {
		body = prettyBody(body, call.Response, rc.color)
	}
	_, err = w.Write(body)
	return err
}

// writeJSONLine encodes the upstream-compatible JSON result of res as one
// line. Every string field goes through runner.Redact, matching the
// reference implementation's json/result.rs (every *Json::from_* builder
// redacts its string fields, including curl_cmd, cookie/header/query
// values and the URL).
func writeJSONLine(sink io.Writer, runner *engine.Runner, res *engine.UnitResult) error {
	jr := toJSONResult(res, runner.Redact)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // `<`, `>` and `&` are written as is
	if err := enc.Encode(jr); err != nil {
		return err
	}
	_, err := sink.Write(buf.Bytes())
	return err
}

// lastEntryAndCall returns the last attempted entry and its last HTTP
// exchange, or (nil, nil) when the file had no calls (a parse error, or an
// entry that never sent a request).
func lastEntryAndCall(res *engine.UnitResult) (*engine.EntryResult, *engine.Call) {
	if len(res.Entries) == 0 {
		return nil, nil
	}
	last := res.Entries[len(res.Entries)-1]
	if len(last.Calls) == 0 {
		return last, nil
	}
	return last, &last.Calls[len(last.Calls)-1]
}

// lastEntryLine is the source line an output/decode failure of res is
// attributed to: the request line of its last attempted entry (1 when the
// file has no entries at all).
func lastEntryLine(res *engine.UnitResult) int {
	if len(res.Entries) == 0 {
		return 1
	}
	return res.Entries[len(res.Entries)-1].Line
}

// fileWriteError reports that path (the -o/--output target) could not be
// written, rendered like the engine's own FileWriteAccess entry error.
func fileWriteError(res *engine.UnitResult, path string, cause error) *outputError {
	e := runerr.New(entrySpan(res), runerr.FileWriteAccess, false)
	e.Value, e.Reason = path, cause.Error()
	return &outputError{rendered: e.Render(res.File, string(res.Source), 0)}
}

// decompressionError reports that the last response's body could not be
// decoded for output (--compressed, or an entry-level `compressed: true`),
// rendered like the engine's own body-decoding errors.
func decompressionError(res *engine.UnitResult, cause *exchange.BodyError) *outputError {
	e := runerr.New(entrySpan(res), runerr.HTTP, false)
	e.Value, e.Reason = cause.Description(), cause.Message()
	return &outputError{rendered: e.Render(res.File, string(res.Source), 0)}
}

// entrySpan is a one-column span at the start of the request line of res's
// last attempted entry, matching where the engine points a whole-entry
// error.
func entrySpan(res *engine.UnitResult) syntax.Span {
	line := lastEntryLine(res)
	return syntax.Span{Start: syntax.Pos{Line: line, Col: 1}, End: syntax.Pos{Line: line, Col: 2}}
}

// identityString is a no-op redact function: -i writes the response
// headers it was asked to include as-is, the same raw form the response
// itself carried (unlike --error-format long, which is a diagnostic sink
// and must never carry a raw secret).
func identityString(s string) string { return s }

// writeStatusAndHeaders prints the status line and headers the way the
// upstream `-i` flag does: "VERSION STATUS\n", one "Name: value\n" per
// header, then a blank line before the body; color wraps the status line
// bold green and each header name bold cyan, matching --color. redact is
// applied to every header value (identity for -i's own raw output; the
// run's secret registry for --error-format long, since that sink must
// never carry a raw secret either).
func writeStatusAndHeaders(w io.Writer, r *exchange.Response, color bool, redact func(string) string) {
	if !color {
		fmt.Fprintf(w, "%s %d\n", r.Version, r.Status) //nolint:errcheck // best-effort stdout write
		for _, h := range r.Headers {
			fmt.Fprintf(w, "%s: %s\n", h.Name, redact(h.Value)) //nolint:errcheck
		}
		fmt.Fprintln(w) //nolint:errcheck
		return
	}
	fmt.Fprintf(w, "\x1b[1;32m%s %d\n\x1b[0m", r.Version, r.Status) //nolint:errcheck
	for _, h := range r.Headers {
		fmt.Fprintf(w, "\x1b[1;36m%s\x1b[0m: %s\n", h.Name, redact(h.Value)) //nolint:errcheck
	}
	fmt.Fprintln(w) //nolint:errcheck
}
