// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
)

// ResponseValidator checks a response against a contract (an OpenAPI
// description). One validator serves every unit of a run: it must be safe
// for concurrent use.
type ResponseValidator interface {
	// ValidateResponse returns the violations of resp, the response to
	// req (the final one, after redirects); none when it conforms.
	ValidateResponse(ctx context.Context, req *exchange.Request, resp *exchange.Response) []Violation
}

// ViolationKind classifies a violation.
type ViolationKind string

// Violation kinds.
const (
	// ViolationUnmatched: no operation of the contract matches the
	// request (a warning outside strict mode).
	ViolationUnmatched ViolationKind = "unmatched"
	// ViolationStatus: the status is not documented.
	ViolationStatus ViolationKind = "status"
	// ViolationHeader: a documented header is missing or invalid.
	ViolationHeader ViolationKind = "header"
	// ViolationContentType: the content type is not documented.
	ViolationContentType ViolationKind = "content-type"
	// ViolationBody: the body does not match its schema.
	ViolationBody ViolationKind = "body"
	// ViolationError: the response could not be checked.
	ViolationError ViolationKind = "error"
)

// NoContract is a ResponseValidator checking nothing: as Job.Validator,
// it turns the run's contract off for one job.
var NoContract ResponseValidator = noContract{}

type noContract struct{}

func (noContract) ValidateResponse(context.Context, *exchange.Request, *exchange.Response) []Violation {
	return nil
}

// Violation is a response not conforming to the contract.
type Violation struct {
	Kind ViolationKind
	// Operation is the matched operation, "GET /pets/{petId}"; empty when
	// no operation matched.
	Operation string
	// SpecPointer locates the broken rule in the contract, as a JSON
	// pointer fragment ("#/paths/~1pets/get/responses/200").
	SpecPointer string
	// InstancePath locates the offending part of the response: a JSON
	// pointer into the body ("/0/name"), or "header <Name>"; empty for the
	// response as a whole.
	InstancePath string
	Message      string
	// Warning marks a finding that does not fail the entry, such as a
	// request no operation matches outside of strict mode.
	Warning bool
}

// ContractEvaluated is sent after the response of an entry was checked
// against the contract. Violations hold raw values.
type ContractEvaluated struct {
	Index      int
	Violations []Violation
}

func (ContractEvaluated) isEvent() {}

// validateContract checks the final call of res against the unit's
// validator: violations fail the entry as asserts do, at the status line
// of the response section (or the request line without one); warnings are
// logged.
func (u *unit) validateContract(ctx context.Context, res *EntryResult, e *syntax.Entry) {
	v := u.io.validator
	if v == nil || len(res.Calls) == 0 {
		return
	}
	last := res.Calls[len(res.Calls)-1]
	vs := v.ValidateResponse(ctx, &last.Request, last.Response)
	u.emit(ContractEvaluated{Index: res.Index, Violations: vs})
	if len(vs) == 0 {
		return
	}
	res.Violations = vs
	span := e.Request.Method.Span
	if e.Response != nil {
		span = e.Response.Version.Span
		span.End = e.Response.Status.Span.End
	}
	for _, vi := range vs {
		if vi.Warning {
			u.log(LogWarning, contractWarning(vi))
			continue
		}
		err := runerr.New(span, runerr.ContractViolation, true)
		err.Name, err.Value, err.Reason = vi.InstancePath, vi.SpecPointer, vi.Message
		res.Asserts = append(res.Asserts, Assert{Line: span.Start.Line, Err: err})
	}
}

func contractWarning(v Violation) string {
	if v.SpecPointer == "" {
		return v.Message
	}
	return v.Message + " (" + v.SpecPointer + ")"
}
