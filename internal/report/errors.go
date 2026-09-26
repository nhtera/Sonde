// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import "github.com/nhtera/sonde/engine"

// renderErrors renders every decisive error of res (those of every
// attempt that was not retried; mirrors engine.UnitResult.Errors) through
// runerr.Error.Render, redacting the result, and splits them into assert
// failures and runtime errors — the grouping JUnit's <failure>/<error>
// children use. internal/report may not import internal/runerr directly
// (architecture.md §2), so the error value is only ever used through the
// engine-declared field/method it came from, never named as a type here.
func renderErrors(res *engine.UnitResult, redact func(string) string) (failures, errs []string) {
	for _, e := range res.Entries {
		if e.Retried {
			continue
		}
		for _, err := range e.Errors {
			msg := redact(err.Render(res.File, string(res.Source), e.Line))
			if err.Assert {
				failures = append(failures, msg)
			} else {
				errs = append(errs, msg)
			}
		}
	}
	return failures, errs
}
