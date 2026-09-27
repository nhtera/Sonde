// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"runtime/debug"
	"strings"
	"sync"
)

const modulePath = "github.com/nhtera/sonde"

// moduleVersion is the version of this module in the running binary,
// without its "v", for the default User-Agent; "" when unknown (a
// development build).
var moduleVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	mod := &info.Main
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			mod = dep
		}
	}
	if mod.Path != modulePath || mod.Version == "(devel)" {
		return ""
	}
	return strings.TrimPrefix(mod.Version, "v")
})
