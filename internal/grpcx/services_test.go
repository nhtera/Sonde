// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"context"
	"io/fs"
	"reflect"
	"testing"
)

func TestServices(t *testing.T) {
	protos := map[string]string{"a.proto": `syntax = "proto3";
package t;
message Req {}
message Res {}
service Zed { rpc Z(Req) returns (Res); }
service Alpha {
  rpc One(Req) returns (Res);
  rpc Watch(Req) returns (stream Res);
  rpc Chat(stream Req) returns (stream Res);
}
`}
	d, err := Files{Protos: []string{"a.proto"}}.Load(context.Background(), func(name string) ([]byte, error) {
		if s, ok := protos[name]; ok {
			return []byte(s), nil
		}
		return nil, fs.ErrNotExist
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Service{
		{Name: "t.Alpha", Methods: []MethodInfo{
			{Name: "One", Path: "/t.Alpha/One", Input: "t.Req", Output: "t.Res"},
			{Name: "Watch", Path: "/t.Alpha/Watch", ServerStreaming: true, Input: "t.Req", Output: "t.Res"},
			{Name: "Chat", Path: "/t.Alpha/Chat", ClientStreaming: true, ServerStreaming: true, Input: "t.Req", Output: "t.Res"},
		}},
		{Name: "t.Zed", Methods: []MethodInfo{{Name: "Z", Path: "/t.Zed/Z", Input: "t.Req", Output: "t.Res"}}},
	}
	if got := d.Services(); !reflect.DeepEqual(got, want) {
		t.Errorf("Services =\n%+v\nwant\n%+v", got, want)
	}
	if _, err := d.Method(want[0].Methods[1].Path); err != nil {
		t.Error(err)
	}
}
