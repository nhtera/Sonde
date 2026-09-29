// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/pflag"

	"github.com/nhtera/sonde/internal/runplan"
)

func addOpenAPIFlags(f *pflag.FlagSet, o *runplan.OpenAPI) {
	f.StringVar(&o.Spec, "openapi", "", "validates every response against an OpenAPI spec (file, or URL with --openapi-allow-remote)")
	f.StringVar(&o.Server, "openapi-server", "", "base URL replacing the spec's servers when matching requests to operations")
	f.BoolVar(&o.Strict, "openapi-strict", false, "fails a request that no operation of the spec matches")
	f.BoolVar(&o.AllowRemote, "openapi-allow-remote", false, "allows a remote spec and remote $ref targets")
}
