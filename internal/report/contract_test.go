// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// echoValidator reports the response body as a violation message.
type echoValidator struct{}

func (echoValidator) ValidateResponse(_ context.Context, _ *exchange.Request, resp *exchange.Response) []engine.Violation {
	return []engine.Violation{
		{Kind: engine.ViolationBody, Operation: "GET /", SpecPointer: "#/paths/~1/get", InstancePath: "/token", Message: "bad value " + string(resp.Body)},
		{Kind: engine.ViolationUnmatched, Message: "unmatched", Warning: true},
	}
}

func TestJSONContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "row-secret")
	}))
	t.Cleanup(srv.Close)
	r := engine.NewRunner(engine.Options{Validator: echoValidator{}})
	var res *engine.UnitResult
	jobs := func(yield func(engine.Job) bool) {
		yield(engine.Job{Name: "a.hurl", Source: []byte("GET " + srv.URL + "\n"), Row: &engine.Row{Index: 1, Secrets: map[string]string{"s": "row-secret"}}})
	}
	r.RunAll(context.Background(), nil, jobs, 1, engine.Hooks{Finished: func(_ int, _ engine.Job, u *engine.UnitResult, err error) bool {
		if err != nil {
			t.Fatal(err)
		}
		res = u
		return true
	}})
	jr, err := JSON(res, r.Redact, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalJSONLine(jr)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `"sonde":{"contract":{"violations":[{"instance_path":"/token","kind":"body","message":"bad value ***","operation":"GET /","spec_pointer":"#/paths/~1/get"},{"kind":"unmatched","message":"unmatched","warning":true}]}},"time":`
	if !strings.Contains(got, want) {
		t.Errorf("JSON lacks %s:\n%s", want, got)
	}
	if strings.Contains(got, "row-secret") {
		t.Error("JSON leaks the row secret")
	}
}
