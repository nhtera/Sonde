// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
)

// pathValidator reports a violation for /json and a warning for /hello.
type pathValidator struct{ seen []string }

func (p *pathValidator) ValidateResponse(_ context.Context, req *exchange.Request, resp *exchange.Response) []Violation {
	p.seen = append(p.seen, req.Method+" "+req.URL+" "+strconv.Itoa(resp.Status/100))
	switch {
	case strings.HasSuffix(req.URL, "/json"):
		return []Violation{{Kind: ViolationBody, Operation: "GET /json", SpecPointer: "#/paths/~1json/get", InstancePath: "/id", Message: "expected string, got integer s3cr3t-token"}}
	case strings.HasSuffix(req.URL, "/hello"):
		return []Violation{{Kind: ViolationUnmatched, Message: "no operation matches GET /hello", Warning: true}}
	}
	return nil
}

func TestContractViolations(t *testing.T) {
	v := &pathValidator{}
	res, rec := run(t, "GET {{base}}/json\nHTTP 200\n[Captures]\nt: jsonpath \"$.token\" redact\n\nGET {{base}}/hello\n", Options{Validator: v, ContinueOnError: true})
	if res.Success {
		t.Fatal("run succeeded despite a contract violation")
	}
	if len(v.seen) != 2 {
		t.Fatalf("validator saw %v", v.seen)
	}
	e1, e2 := res.Entries[0], res.Entries[1]
	if len(e1.Errors) != 1 || e1.Errors[0].Kind != runerr.ContractViolation || !e1.Errors[0].Assert || e1.Errors[0].Span.Start.Line != 2 {
		t.Fatalf("entry 1 errors = %+v", e1.Errors)
	}
	msg := res.Redact(e1.Errors[0].Render(res.File, string(res.Source), 1))
	for _, want := range []string{"Contract violation", "expected string, got integer ***", "at: /id", "spec: #/paths/~1json/get", " 2 | HTTP 200\n   | ^^^^^^^^"} {
		if !strings.Contains(msg, want) {
			t.Errorf("rendered error lacks %q:\n%s", want, msg)
		}
	}
	if len(e1.Violations) != 1 {
		t.Errorf("entry 1 violations = %+v", e1.Violations)
	}
	if len(e2.Errors) != 0 || len(e2.Violations) != 1 || !e2.Violations[0].Warning {
		t.Errorf("entry 2 = errors %+v, violations %+v", e2.Errors, e2.Violations)
	}
	if w := rec.text(LogWarning); !strings.Contains(w, "no operation matches GET /hello") {
		t.Errorf("warnings = %q", w)
	}
}

func TestContractSkippedWithNoAssert(t *testing.T) {
	v := &pathValidator{}
	res, _ := run(t, "GET {{base}}/json\n", Options{Validator: v, NoAssert: true})
	if !res.Success || len(v.seen) != 0 {
		t.Errorf("success %v, validator saw %v", res.Success, v.seen)
	}
}

func TestContractJobValidator(t *testing.T) {
	srv := server(t)
	run, job := &pathValidator{}, &pathValidator{}
	r := NewRunner(Options{Validator: run})
	jobs := func(yield func(Job) bool) {
		if yield(Job{Name: t.TempDir() + "/a.hurl", Source: []byte("GET " + srv.URL + "/lines\n")}) {
			yield(Job{Name: t.TempDir() + "/b.hurl", Source: []byte("GET " + srv.URL + "/lines\n"), Validator: job})
		}
	}
	r.RunAll(context.Background(), nil, jobs, 1, Hooks{})
	if len(run.seen) != 1 || len(job.seen) != 1 {
		t.Errorf("run validator saw %v, job validator saw %v", run.seen, job.seen)
	}
}
