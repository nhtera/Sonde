// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package view

import (
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/report"
)

// ConvertEntry projects a finished attempt: the report's entry shape
// (redacted with redact), each call's decoded and redacted body stored in
// bodies, and its errors.
func ConvertEntry(file string, e *engine.EntryResult, redact Redactor, bodies BodyStore) Entry {
	unit := &engine.UnitResult{File: file, Entries: []*engine.EntryResult{e}}
	out := Entry{Bodies: []Body{}, Timings: []Timings{}, Errors: []Error{}, Success: len(e.Errors) == 0, Retried: e.Retried}
	if r, err := report.JSON(unit, redact, nil); err == nil && len(r.Entries) == 1 {
		out.Entry = r.Entries[0]
		maskEntryCookies(&out.Entry)
	} else {
		out.Index, out.Line = e.Index, e.Line
	}
	for _, call := range e.Calls {
		out.Bodies = append(out.Bodies, storeBody(call, redact, bodies))
		t := call.Timings
		out.Timings = append(out.Timings, Timings{
			NameLookup: t.NameLookup.Microseconds(), Connect: t.Connect.Microseconds(), AppConnect: t.AppConnect.Microseconds(),
			PreTransfer: t.PreTransfer.Microseconds(), StartTransfer: t.StartTransfer.Microseconds(), Total: t.Total.Microseconds(),
		})
	}
	for _, err := range e.Errors {
		out.Errors = append(out.Errors, ConvertError(err, redact))
	}
	return out
}

// ConvertError projects an engine error.
func ConvertError(err *engine.Error, redact Redactor) Error {
	span := err.Span()
	return Error{
		Line: span.Start.Line, Column: span.Start.Col,
		Kind: string(err.Kind()), Assert: err.Assert(),
		Description: redact(err.Description()), Message: redact(err.Message()),
		Transport: enginex.Transport(err),
	}
}

// storeBody decodes, redacts (every byte, text or binary) and stores a
// call's response body. The transferred encoding never reaches the page.
func storeBody(call engine.Call, redact Redactor, bodies BodyStore) Body {
	resp := call.Response
	if resp == nil || len(resp.Body) == 0 {
		return Body{}
	}
	ct, _ := resp.ContentType()
	body := Body{ContentType: ct}
	data, err := resp.DecodedBody()
	if err != nil {
		// A body that does not decode is not stored at all: redaction
		// cannot see a secret inside compressed bytes.
		body.Error, body.Size = redact(err.Error()), len(resp.Body)
		return body
	}
	redacted := RedactBytes(data, redact)
	body.Size = len(redacted)
	if bodies != nil {
		body.ID = bodies.Put(redacted, data, ct)
	}
	return body
}
