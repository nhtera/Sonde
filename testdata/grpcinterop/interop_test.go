// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package grpcinterop runs Sonde's gRPC calls against grpc-go servers. It
// is a separate module so that grpc-go never enters Sonde's own build:
// run it with `make test-grpc-interop`.
package grpcinterop

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // registers the gzip compressor
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	rv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	rv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/nhtera/sonde/engine"
)

const protoDir = "../../engine/testdata/grpc"

// files compiles the test protos into a registry.
func files(t *testing.T) (*protoregistry.Files, protoreflect.FileDescriptor) {
	t.Helper()
	c := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: []string{filepath.Join(protoDir, "protos")},
	})}
	fds, err := c.Compile(context.Background(), "sonde/test/greeter.proto")
	if err != nil {
		t.Fatal(err)
	}
	reg := &protoregistry.Files{}
	var add func(fd protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if _, err := reg.FindFileByPath(fd.Path()); err == nil {
			return
		}
		for i := range fd.Imports().Len() {
			add(fd.Imports().Get(i).FileDescriptor)
		}
		if err := reg.RegisterFile(fd); err != nil {
			t.Fatal(err)
		}
	}
	add(fds[0])
	return reg, fds[0]
}

// greeter implements sonde.test.Greeter with dynamic messages.
func greeter(fd protoreflect.FileDescriptor) grpc.StreamHandler {
	svc := fd.Services().ByName("Greeter")
	return func(_ any, stream grpc.ServerStream) error {
		full, _ := grpc.MethodFromServerStream(stream)
		md := svc.Methods().ByName(protoreflect.Name(full[strings.LastIndex(full, "/")+1:]))
		if md == nil {
			return status.Error(codes.Unimplemented, "unknown method "+full)
		}
		req := dynamicpb.NewMessage(md.Input())
		if err := stream.RecvMsg(req); err != nil {
			return err
		}
		in, _ := metadata.FromIncomingContext(stream.Context())
		_ = stream.SetHeader(metadata.Pairs("x-echo", strings.Join(in.Get("x-request-id"), ",")))
		switch md.Name() {
		case "SayHello":
			name := req.Get(md.Input().Fields().ByName("name")).String()
			switch name {
			case "missing":
				return status.Error(codes.NotFound, "user "+name+" not found")
			case "slow":
				<-stream.Context().Done()
				return stream.Context().Err()
			case "gzip":
				_ = grpc.SetSendCompressor(stream.Context(), "gzip")
			}
			reply := dynamicpb.NewMessage(md.Output())
			reply.Set(md.Output().Fields().ByName("message"), protoreflect.ValueOfString("Hello "+name))
			stream.SetTrailer(metadata.Pairs("x-trailer", "t"))
			return stream.SendMsg(reply)
		case "Count":
			n := req.Get(md.Input().Fields().ByName("n")).Int()
			for i := range n {
				reply := dynamicpb.NewMessage(md.Output())
				reply.Set(md.Output().Fields().ByName("i"), protoreflect.ValueOfInt32(int32(i)))
				if err := stream.SendMsg(reply); err != nil {
					return err
				}
			}
			return nil
		}
		return status.Error(codes.Unimplemented, "not implemented")
	}
}

// serve starts a grpc-go server: TLS or cleartext, with reflection
// version "v1", "v1alpha" or none.
func serve(t *testing.T, useTLS bool, version string) string {
	t.Helper()
	reg, fd := files(t)
	var opts []grpc.ServerOption
	if useTLS {
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{selfSigned(t)}})))
	}
	opts = append(opts, grpc.UnknownServiceHandler(greeter(fd)))
	s := grpc.NewServer(opts...)
	ropts := reflection.ServerOptions{Services: s, DescriptorResolver: reg}
	switch version {
	case "v1":
		rv1.RegisterServerReflectionServer(s, reflection.NewServerV1(ropts))
	case "v1alpha":
		rv1alpha.RegisterServerReflectionServer(s, reflection.NewServer(ropts))
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	scheme := "http://"
	if useTLS {
		scheme = "https://"
	}
	return scheme + lis.Addr().String()
}

func run(t *testing.T, base, src string) *engine.UnitResult {
	t.Helper()
	opt := engine.Options{Variables: map[string]any{"base": base}}
	opt.HTTP.Insecure = true
	res, err := engine.NewRunner(opt).RunSource(context.Background(), filepath.Join(protoDir, "t.sonde"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if res.ParseError != nil {
		t.Fatal(res.ParseError)
	}
	return res
}

const calls = `POST {{base}}/sonde.test.Greeter/SayHello
x-request-id: 7
[SondeGrpc]
{"name": "grpc-go"}
HTTP 200
[Asserts]
sondeGrpc == "OK"
header "x-echo" == "7"
header "x-trailer" == "t"
jsonpath "$.message" == "Hello grpc-go"

POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
{"name": "gzip"}
HTTP 200
[Asserts]
header "grpc-encoding" == "gzip"
jsonpath "$.message" == "Hello gzip"

POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
{"name": "missing"}
HTTP 200
[Asserts]
sondeGrpc == "NOT_FOUND"
sondeGrpc "message" == "user missing not found"

POST {{base}}/sonde.test.Greeter/Count
[SondeGrpc]
{"n": 3}
HTTP 200
[Asserts]
sondeGrpc == "OK"
sondeStream count == 3
sondeStream nth 2 jsonpath "$.i" == 2
`

func TestInterop(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tls     bool
		version string
	}{{"h2c v1", false, "v1"}, {"tls v1alpha", true, "v1alpha"}} {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, serve(t, tc.tls, tc.version), calls)
			if !res.Success {
				for _, e := range res.Errors() {
					t.Error(e.Render())
				}
			}
		})
	}
}

func TestInteropProtoFiles(t *testing.T) {
	res := run(t, serve(t, false, ""), `POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": "files"}
HTTP 200
[Asserts]
jsonpath "$.message" == "Hello files"
`)
	if !res.Success {
		t.Fatal(res.Errors())
	}
}

func TestInteropDeadline(t *testing.T) {
	res := run(t, serve(t, false, "v1"), `POST {{base}}/sonde.test.Greeter/SayHello
grpc-timeout: 200m
[SondeGrpc]
{"name": "slow"}
`)
	errs := res.Errors()
	if len(errs) != 1 || errs[0].Kind() != engine.ErrorGRPC || !strings.Contains(errs[0].Error(), "DEADLINE_EXCEEDED: the deadline") {
		t.Fatalf("errors: %v", errs)
	}
}
