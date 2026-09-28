// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Request encodes the JSON of a request message (canonical proto3 JSON;
// empty for the empty message) in the protobuf format.
func (d *Descriptors) Request(m *Method, data []byte) ([]byte, error) {
	msg := dynamicpb.NewMessage(m.desc.Input())
	if len(bytes.TrimSpace(data)) > 0 {
		opts := protojson.UnmarshalOptions{Resolver: d.types}
		if err := opts.Unmarshal(data, msg); err != nil {
			return nil, fmt.Errorf("the request is not a valid %s message: %w", m.desc.Input().FullName(), err)
		}
	}
	return proto.Marshal(msg)
}

// Reply decodes a reply message into compact JSON: canonical proto3 JSON,
// fields holding their default value omitted.
func (d *Descriptors) Reply(m *Method, data []byte) ([]byte, error) {
	msg := dynamicpb.NewMessage(m.desc.Output())
	if err := (proto.UnmarshalOptions{Resolver: d.types}).Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("the reply is not a valid %s message: %w", m.desc.Output().FullName(), err)
	}
	out, err := protojson.MarshalOptions{Resolver: d.types}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("could not write the reply as JSON: %w", err)
	}
	// protojson varies its whitespace on purpose: compact it so the body
	// is stable.
	var buf bytes.Buffer
	if err := json.Compact(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
