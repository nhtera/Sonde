// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// grpcStatus evaluates sondeGrpc: one field of the call's status, none
// when the entry is not a gRPC call or its stream stopped before the
// status arrived.
func grpcStatus(q *syntax.Query, s *exchange.GRPCStatus) value.Value {
	if s == nil {
		return nil
	}
	field := "status"
	if f, ok := q.Arg.(*syntax.StreamField); ok {
		field = f.Name
	}
	switch field {
	case "code":
		return value.Int(s.Code)
	case "message":
		return value.String(s.Message)
	}
	return value.String(s.Status)
}
