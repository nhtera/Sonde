// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
)

func TestJSONGolden(t *testing.T) {
	cases := []struct {
		name   string
		result *engine.UnitResult
	}{
		{"success", successResult("tests/success.hurl")},
		{"failure", failureResult("tests/failure.hurl")},
		{"error", errorResult("tests/error.hurl")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			jr, err := JSON(c.result, redactTestSecret, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := MarshalJSONLine(jr)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			assertGolden(t, "../../testdata/report/json_"+c.name+".golden", got)
		})
	}
}

// TestJSON_Redacts checks that the run's secret never survives into a
// Result, in any field it could reach: cookie value, header value, URL,
// curl command, response body reference aside (body itself is not
// embedded, only its store path) and rendered assert message.
func TestJSON_Redacts(t *testing.T) {
	for _, res := range []*engine.UnitResult{
		successResult("t.hurl"), failureResult("t.hurl"), errorResult("t.hurl"),
	} {
		jr, err := JSON(res, redactTestSecret, nil)
		if err != nil {
			t.Fatal(err)
		}
		line, err := MarshalJSONLine(jr)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(line), testSecret) {
			t.Errorf("secret leaked into JSON result for %s:\n%s", res.File, line)
		}
	}
}
