// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import "fmt"

// invalidFlagErr reports a flag value the command line gave that failed to
// parse: a usage error (ExitUsage). placeholder is the flag's value name as
// it appears in its own usage line (e.g. "NUM", "SECONDS"), matching the
// upstream CLI's own message shape.
func invalidFlagErr(flag, placeholder, value string, cause error) error {
	return NewExitError(ExitUsage, fmt.Errorf("invalid value '%s' for '--%s <%s>' (%v)", value, flag, placeholder, cause))
}

// unsupportedOptionErr reports an option sonde does not implement: a
// runtime error (ExitRuntime), matching docs/architecture.md's contract.
func unsupportedOptionErr(name string) error {
	return NewExitError(ExitRuntime, fmt.Errorf("option %q is not supported by sonde yet", name))
}
