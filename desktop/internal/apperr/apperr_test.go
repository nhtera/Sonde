// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestMarshal(t *testing.T) {
	if got := string(Marshal(fmt.Errorf("x: %w", New(Conflict, "changed")))); got != `{"code":"conflict","message":"changed"}` {
		t.Errorf("wrapped: %s", got)
	}
	if got := string(Marshal(errors.New("boom"))); got != `{"code":"error","message":"boom"}` {
		t.Errorf("plain: %s", got)
	}
	e := &Error{Code: Conflict, Message: "m", Data: map[string]string{"hash": "h"}}
	if got := string(Marshal(e)); got != `{"code":"conflict","message":"m","data":{"hash":"h"}}` {
		t.Errorf("data: %s", got)
	}
}
