// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// http.Transport keeps a background connection-reaping goroutine
		// alive briefly after CloseIdleConnections; not a leak we cause.
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}
