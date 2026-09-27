// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// streamFields are the sondeStream fields of each protocol.
var streamFields = map[exchange.Protocol]map[string]bool{
	exchange.ProtocolSSE:       {"data": true, "event": true, "id": true, "retry": true},
	exchange.ProtocolWebSocket: {"data": true, "type": true},
}

var protocolNames = map[exchange.Protocol]string{exchange.ProtocolSSE: "an SSE", exchange.ProtocolWebSocket: "a WebSocket"}

// stream evaluates sondeStream: one field of each message received, in
// order. A response without a stream returns nothing.
func stream(q *syntax.Query, s *exchange.Stream) (value.Value, error) {
	if s == nil {
		return nil, nil
	}
	field := "data"
	if f, ok := q.Arg.(*syntax.StreamField); ok {
		field = f.Name
	}
	if !streamFields[s.Protocol][field] {
		e := runerr.New(q.Span, runerr.Stream, false)
		e.Value = "Invalid sondeStream field"
		e.Reason = "the field <" + field + "> does not apply to " + protocolNames[s.Protocol] + " stream"
		return nil, e
	}
	list := value.List{}
	for _, m := range s.ReceivedMessages() {
		var v value.Value
		switch field {
		case "data":
			if m.Binary {
				v = value.Bytes(m.Data)
			} else {
				v = value.String(m.Data)
			}
		case "event":
			v = value.String(m.Event)
		case "id":
			v = value.String(m.ID)
		case "retry":
			v = value.Null{}
			if m.Retry != nil {
				v = value.Int(*m.Retry)
			}
		case "type":
			v = value.String("text")
			if m.Binary {
				v = value.String("binary")
			}
		}
		list = append(list, v)
	}
	return list, nil
}
