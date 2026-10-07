// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/nhtera/sonde/exchange"
)

// runGRPC runs src as a .sonde file of dir (testdata/grpc by default) with
// {{base}} set to srv.
func runGRPC(t *testing.T, srv *grpcTestServer, dir, src string, opt Options) (*UnitResult, *streamRecorder) {
	t.Helper()
	if dir == "" {
		dir = "testdata/grpc"
	}
	if opt.Variables == nil {
		opt.Variables = map[string]any{}
	}
	opt.Variables["base"] = srv.URL
	if srv.TLS != nil {
		opt.HTTP.Insecure = true
	}
	rec := &streamRecorder{}
	opt.OnEvent = rec.on
	res, err := NewRunner(opt).RunSource(context.Background(), filepath.Join(dir, "t.sonde"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if res.ParseError != nil {
		t.Fatal(res.ParseError)
	}
	return res, rec
}

func TestRunGRPCUnaryProto(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	res, rec := runGRPC(t, srv, "", `POST {{base}}/sonde.test.Greeter/SayHello
x-request-id: 42
[Options]
max-time: 5s
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": "sonde", "times": 3}
HTTP 200
[Captures]
greeting: jsonpath "$.message"
[Asserts]
sondeGrpc == "OK"
sondeGrpc "code" == 0
sondeGrpc "message" == ""
header "grpc-status" == "0"
header "x-echo" == "42"
header "x-timeout" == "5S"
jsonpath "$.message" == "Hello sonde"
jsonpath "$.times" == 3
jsonpath "$.mood" == "HAPPY"
jsonpath "$.big" == "1152921504606846976"
jsonpath "$.extra['@type']" == "type.googleapis.com/sonde.test.HelloRequest"
jsonpath "$.extra.name" == "sonde"

POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": ""}
HTTP 200
[Asserts]
body == "{\"message\":\"Hello \",\"mood\":\"HAPPY\",\"extra\":{\"@type\":\"type.googleapis.com/sonde.test.HelloRequest\"},\"big\":\"1152921504606846976\"}"
jsonpath "$.times" not exists
`, Options{Verbosity: Verbose})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
	e := res.Entries[0]
	call := e.Calls[0]
	if string(call.Request.Body) != `{"name": "sonde", "times": 3}` {
		t.Errorf("request body = %q", call.Request.Body)
	}
	if call.Response.Version != "HTTP/2" || call.Response.GRPC == nil || call.Response.GRPC.Status != "OK" {
		t.Errorf("response = %s %+v", call.Response.Version, call.Response.GRPC)
	}
	if e.Curl != "" {
		t.Errorf("curl = %q", e.Curl)
	}
	if rec.received != 0 {
		t.Errorf("received events: %d", rec.received)
	}
}

func TestRunGRPCErrors(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	res, _ := runGRPC(t, srv, "", `POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": "missing"}
HTTP 200
[Asserts]
sondeGrpc == "NOT_FOUND"
sondeGrpc "code" == 5
sondeGrpc "message" == "user missing not found: 100%"
body == ""

POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": "proxy"}
HTTP 503
[Asserts]
sondeGrpc == "UNAVAILABLE"
sondeGrpc "message" == "HTTP status 503 without grpc-status"
body == "upstream unavailable"

POST {{base}}/sonde.test.Greeter/SayHello
grpc-timeout: 3S
[SondeGrpc]
proto: protos/sonde/test/greeter.proto
import-path: protos
{"name": "x"}
HTTP 200
[Asserts]
header "x-timeout" == "3S"
`, Options{})
	if !res.Success {
		t.Fatalf("expected status: %v", res.Errors())
	}

	for _, tc := range []struct {
		name, src, want string
		kind            ErrorKind
	}{
		{"unchecked status", `{"name": "missing"}
HTTP 200`, "gRPC status: NOT_FOUND: user missing not found: 100%", ErrorGRPC},
		{"reset", `{"name": "reset"}`, "INTERNAL: the HTTP/2 stream was reset (INTERNAL_ERROR)", ErrorGRPC},
		{"html page", `{"name": "html"}`, `UNKNOWN: the response is not gRPC (content type "text/html")`, ErrorGRPC},
		{"two replies", `{"name": "twice"}`, "the server replied with 2 messages to a unary call", ErrorGRPC},
		{"large reply", `{"name": "big"}`, "more than the limit of 4194304", ErrorGRPC},
		{"bad request", `{"nom": "x"}`, `unknown field "nom"`, ErrorGRPC},
		{"timeout", `{"name": "slow"}
[Options]
max-time: 300ms`, "timed out", ErrorHTTP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n" + tc.src + "\n"
			if strings.Contains(tc.src, "[Options]") {
				body, opts, _ := strings.Cut(tc.src, "\n")
				src = "POST {{base}}/sonde.test.Greeter/SayHello\n" + opts + "\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n" + body + "\n"
			}
			res, _ := runGRPC(t, srv, "", src, Options{})
			errs := res.Errors()
			if res.Success || len(errs) != 1 || errs[0].Kind() != tc.kind || !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("errors: %v", errs)
			}
		})
	}
}

func TestRunGRPCInvalidEntries(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	for _, tc := range []struct{ name, src, want string }{
		{"method", "GET {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "must use POST"},
		{"path", "POST {{base}}/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "is not /package.Service/Method"},
		{"header", "POST {{base}}/sonde.test.Greeter/SayHello\ngrpc-encoding: gzip\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "grpc-encoding is set by sonde"},
		{"content type", "POST {{base}}/sonde.test.Greeter/SayHello\nContent-Type: application/json\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "Content-Type is set by sonde"},
		{"form", "POST {{base}}/sonde.test.Greeter/SayHello\n[Form]\na: b\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "not [Form] or [Multipart]"},
		{"service", "POST {{base}}/sonde.test.Nope/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "service sonde.test.Nope is not in the descriptors"},
		{"method name", "POST {{base}}/sonde.test.Greeter/Nope\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "has no method Nope"},
		{"client streaming", "POST {{base}}/sonde.test.Greeter/Chat\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "client-streaming and bidirectional"},
		{"missing file", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/none.proto\n", "could not compile the proto files"},
		{"no import path", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "sonde/test/types.proto"},
		{"outside root", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nprotoset: ../../go.mod\n", "denied"},
		{"no reflection", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\n", "tried grpc.reflection.v1, grpc.reflection.v1alpha"},
		{"two protocols", "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n[SondeMessages]\nreceive\n", "do not go together"},
		{"bad timeout", "POST {{base}}/sonde.test.Greeter/SayHello\ngrpc-timeout: +5S\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\n", "invalid grpc-timeout"},
		{"h2c proxy", "POST {{base}}/sonde.test.Greeter/SayHello\n[Options]\nproxy: http://127.0.0.1:9\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "cannot go through the HTTP proxy"},
		{"http1.1", "POST {{base}}/sonde.test.Greeter/SayHello\n[Options]\nhttp1.1: true\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "the http1.1 option does not apply"},
		{"http1.0", "POST {{base}}/sonde.test.Greeter/SayHello\n[Options]\nhttp1.0: true\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "the http1.0 option does not apply"},
		{"http3", "POST {{base}}/sonde.test.Greeter/SayHello\n[Options]\nhttp3: true\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n", "the http3 option does not apply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := runGRPC(t, srv, "", tc.src, Options{})
			errs := res.Errors()
			if res.Success || len(errs) != 1 || !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("errors: %v", errs)
			}
		})
	}
}

func TestRunGRPCReflectionTLS(t *testing.T) {
	for _, version := range []string{"v1", "v1alpha"} {
		t.Run(version, func(t *testing.T) {
			srv := newGRPCServer(t, true, version)
			res, _ := runGRPC(t, srv, t.TempDir(), `POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
{"name": "tls"}
HTTP 200
[Asserts]
jsonpath "$.message" == "Hello tls"
jsonpath "$.extra.name" == "tls"

POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
{"name": "gzip"}
HTTP 200
[Asserts]
header "grpc-encoding" == "gzip"
jsonpath "$.message" == "Hello gzip"
`, Options{})
			if !res.Success {
				t.Fatalf("errors: %v", res.Errors())
			}
		})
	}
}

func TestRunGRPCProtoset(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	dir := t.TempDir()
	set := &descriptorpb.FileDescriptorSet{}
	var add func(fd protoreflect.FileDescriptor)
	seen := map[string]bool{}
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		for i := range fd.Imports().Len() {
			add(fd.Imports().Get(i).FileDescriptor)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(fd))
	}
	add(srv.files)
	b, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeter.protoset"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := runGRPC(t, srv, dir, `POST {{base}}/sonde.test.Greeter/SayHello
[SondeGrpc]
protoset: {{set}}
{"name": "set"}
HTTP 200
[Asserts]
jsonpath "$.message" == "Hello set"
`, Options{Variables: map[string]any{"set": "greeter.protoset"}})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
}

func TestRunGRPCServerStreaming(t *testing.T) {
	srv := newGRPCServer(t, false, "v1")
	res, rec := runGRPC(t, srv, t.TempDir(), `POST {{base}}/sonde.test.Greeter/Count
[SondeGrpc]
{"n": 3}
HTTP 200
[Asserts]
sondeGrpc == "OK"
sondeStream count == 3
sondeStream nth 2 jsonpath "$.i" == 2
jsonpath "$[1].i" == 1
body == "[{},{\"i\":1},{\"i\":2}]"

POST {{base}}/sonde.test.Greeter/Count
[Options]
sonde-stream-count: 2
[SondeGrpc]
{"n": 100, "delay": 50}
HTTP 200
[Asserts]
sondeStream count == 2
sondeGrpc not exists
`, Options{})
	if !res.Success {
		t.Fatalf("errors: %v", res.Errors())
	}
	s := res.Entries[0].Calls[0].Response.Stream
	if s == nil || s.Protocol != exchange.ProtocolGRPC || s.StopReason != exchange.StopClosed {
		t.Errorf("stream = %+v", s)
	}
	if s := res.Entries[1].Calls[0].Response; s.Stream.StopReason != exchange.StopCount || s.GRPC != nil {
		t.Errorf("stopped stream = %+v, status %+v", s.Stream, s.GRPC)
	}
	if rec.received != 5 {
		t.Errorf("received events: %d", rec.received)
	}
}

func TestRunGRPCCancel(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	res, err := NewRunner(Options{Variables: map[string]any{"base": srv.URL}}).RunSource(ctx, filepath.Join("testdata/grpc", "t.sonde"),
		[]byte("POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n{\"name\": \"slow\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || time.Since(start) > time.Second {
		t.Fatalf("success %v after %v", res.Success, time.Since(start))
	}
}

// TestRunGRPCDescriptorCache checks a Runner reloads edited proto files.
func TestRunGRPCDescriptorCache(t *testing.T) {
	srv := newGRPCServer(t, false, "")
	dir := t.TempDir()
	src := "POST {{base}}/sonde.test.Greeter/SayHello\n[SondeGrpc]\nproto: protos/sonde/test/greeter.proto\nimport-path: protos\n{\"name\": \"a\"}\n"
	for _, name := range []string{"greeter.proto", "types.proto"} {
		b, err := os.ReadFile(filepath.Join("testdata/grpc/protos/sonde/test", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "protos/sonde/test"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "protos/sonde/test", name), b, 0o600); err != nil { //nolint:gosec // G703: test paths
			t.Fatal(err)
		}
	}
	runner := NewRunner(Options{Variables: map[string]any{"base": srv.URL}})
	run := func() *UnitResult {
		res, err := runner.RunSource(context.Background(), filepath.Join(dir, "t.sonde"), []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := run(); !res.Success {
		t.Fatalf("first run: %v", res.Errors())
	}
	path := filepath.Join(dir, "protos/sonde/test/greeter.proto")
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), "rpc SayHello", "rpc Hello", 1)), 0o600); err != nil { //nolint:gosec // G703: test paths
		t.Fatal(err)
	}
	if res := run(); res.Success || !strings.Contains(res.Errors()[0].Error(), "has no method SayHello") {
		t.Fatalf("second run: %v", res.Errors())
	}
}
