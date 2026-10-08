// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import "github.com/nhtera/sonde/engine"

// renderErrors renders every decisive error of res (see
// engine.UnitResult.Decisive; mirrors engine.UnitResult.Errors),
// redacting the result, and splits them into assert failures and runtime
// errors — the grouping JUnit's <failure>/<error> children use.
func renderErrors(res *engine.UnitResult, redact func(string) string) (failures, errs []string) {
	redact = forResult(res, redact)
	for i, e := range res.Entries {
		if !res.Decisive(i) {
			continue
		}
		for _, err := range e.Errors {
			msg := redact(err.Render())
			if err.Assert() {
				failures = append(failures, msg)
			} else {
				errs = append(errs, msg)
			}
		}
	}
	return failures, errs
}

// forResult extends redact with the secrets of res's data row, which the
// run-wide redact does not know: res.Redact masks the run's and the row's
// secrets in one pass, then redact applies as given.
func forResult(res *engine.UnitResult, redact func(string) string) func(string) string {
	if res.Row == 0 {
		return redact
	}
	return func(s string) string { return redact(res.Redact(s)) }
}
