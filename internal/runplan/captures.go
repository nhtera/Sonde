// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"maps"

	"github.com/nhtera/sonde/engine"
)

// ApplyCaptures layers the captures of earlier entries over a run, the way
// a front end reruns one entry of a file after a full run (the desktop's
// Send): plain captures join Options.Variables and `redact` captures
// Options.Secrets, above every other source. A plain capture removes its
// name from the secrets of opts and job, so it is not masked; a `redact`
// capture removes its name from opts.Variables. opts and job are updated
// in place; their maps are copied first, never shared with the caller's.
// A data file must be opened with the `redact` capture names among its
// overridden names, so that a row does not report them as clashing
// secrets.
func ApplyCaptures(opts *engine.Options, job *engine.Job, plain map[string]any, secret map[string]string) {
	opts.Variables = maps.Clone(opts.Variables)
	opts.Secrets = maps.Clone(opts.Secrets)
	job.Secrets = maps.Clone(job.Secrets)
	if opts.Variables == nil {
		opts.Variables = map[string]any{}
	}
	if opts.Secrets == nil {
		opts.Secrets = map[string]string{}
	}
	for name, v := range plain {
		opts.Variables[name] = v
		delete(opts.Secrets, name)
		delete(job.Secrets, name)
	}
	for name, v := range secret {
		opts.Secrets[name] = v
		delete(opts.Variables, name)
	}
}
