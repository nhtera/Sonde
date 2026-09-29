// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/enginex"
)

// TestGRPCDescriptors loads the descriptors of a [SondeGrpc] entry from its
// files, with the job's variables, and never uses reflection.
func TestGRPCDescriptors(t *testing.T) {
	name := filepath.Join("testdata", "grpc", "t.sonde")
	src := "GET http://localhost/other\n\nPOST http://localhost/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: {{proto}}\nimport-path: protos\n{\"name\": \"x\"}\n\n" +
		"POST http://localhost/sonde.test.Greeter/SayHello\n[SondeGrpc]\n{\"name\": \"x\"}\n"
	job := Job{Name: name, Source: []byte(src), Variables: map[string]any{"proto": "protos/sonde/test/greeter.proto"}}
	r := NewRunner(Options{})
	ctx := context.Background()

	d, err := enginex.GRPCDescriptors(ctx, r, job, 2)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("no descriptors")
	}
	if _, err := d.Method("/sonde.test.Greeter/SayHello"); err != nil {
		t.Error(err)
	}
	if d, err := enginex.GRPCDescriptors(ctx, r, job, 3); d != nil || err != nil {
		t.Errorf("reflection entry: %v %v", d, err)
	}
	if _, err := enginex.GRPCDescriptors(ctx, r, job, 1); err == nil || !strings.Contains(err.Error(), "not a gRPC entry") {
		t.Errorf("HTTP entry: %v", err)
	}
	if _, err := enginex.GRPCDescriptors(ctx, r, job, 4); err == nil {
		t.Error("entry out of range: no error")
	}
	job.Variables = map[string]any{"proto": "protos/none.proto"}
	if _, err := enginex.GRPCDescriptors(ctx, r, job, 2); err == nil || !strings.Contains(err.Error(), "could not compile") {
		t.Errorf("missing file: %v", err)
	}
}
