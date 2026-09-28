// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
)

// grpcTestServer is a gRPC server written on net/http for the tests: the
// sonde.test.Greeter service of testdata/grpc and server reflection.
type grpcTestServer struct {
	*httptest.Server
	files protoreflect.FileDescriptor
	// reflection is the reflection version served: "v1", "v1alpha" or "".
	reflection string
}

// compileTestProtos compiles the test protos.
func compileTestProtos(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	c := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: []string{"testdata/grpc/protos"},
	})}
	files, err := c.Compile(context.Background(), "sonde/test/greeter.proto")
	if err != nil {
		t.Fatal(err)
	}
	return files[0]
}

// newGRPCServer starts a cleartext HTTP/2 (h2c) server, or a TLS one.
func newGRPCServer(t *testing.T, tls bool, reflection string) *grpcTestServer {
	t.Helper()
	s := &grpcTestServer{files: compileTestProtos(t), reflection: reflection}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	if tls {
		s.EnableHTTP2 = true
		s.StartTLS()
	} else {
		s.Config.Protocols = new(http.Protocols)
		s.Config.Protocols.SetUnencryptedHTTP2(true)
		s.Start()
	}
	t.Cleanup(s.Close)
	return s
}

func (s *grpcTestServer) message(name string) *dynamicpb.Message {
	d := s.files.Messages().ByName(protoreflect.Name(name))
	if d == nil {
		d = s.files.Imports().Get(1).Messages().ByName(protoreflect.Name(name))
	}
	return dynamicpb.NewMessage(d)
}

func (s *grpcTestServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor != 2 || r.Header.Get("Content-Type") != "application/grpc" || r.Header.Get("Te") != "trailers" {
		http.Error(w, "not a gRPC request", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("X-Timeout", r.Header.Get("Grpc-Timeout"))
	w.Header().Set("X-Echo", r.Header.Get("X-Request-Id"))
	body, _ := io.ReadAll(r.Body)
	msg := []byte{}
	if len(body) >= 5 {
		msg = body[5:]
	}
	switch r.URL.Path {
	case "/sonde.test.Greeter/SayHello":
		s.sayHello(w, r, msg)
	case "/sonde.test.Greeter/Count":
		req := s.message("CountRequest")
		_ = proto.Unmarshal(msg, req)
		n := req.Get(req.Descriptor().Fields().ByName("n")).Int()
		delay := time.Duration(req.Get(req.Descriptor().Fields().ByName("delay")).Int()) * time.Millisecond
		for i := range n {
			reply := s.message("CountReply")
			reply.Set(reply.Descriptor().Fields().ByName("i"), protoreflect.ValueOfInt32(int32(i)))
			writeMessage(w, reply, false)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(delay):
			}
		}
		trailers(w, 0, "")
	case "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":
		s.reflect(w, msg, "v1")
	case "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		s.reflect(w, msg, "v1alpha")
	default:
		trailersOnly(w, 12, "unknown method "+r.URL.Path)
	}
}

func (s *grpcTestServer) sayHello(w http.ResponseWriter, r *http.Request, msg []byte) {
	req := s.message("HelloRequest")
	if err := proto.Unmarshal(msg, req); err != nil {
		trailersOnly(w, 13, err.Error())
		return
	}
	fields := req.Descriptor().Fields()
	name := req.Get(fields.ByName("name")).String()
	switch name {
	case "missing":
		trailersOnly(w, 5, "user "+name+" not found: 100%")
		return
	case "reset":
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	case "proxy":
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream unavailable"))
		return
	case "html":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>hi</p>"))
		return
	case "big":
		reply := s.message("HelloReply")
		reply.Set(reply.Descriptor().Fields().ByName("message"), protoreflect.ValueOfString(strings.Repeat("x", 5<<20)))
		writeMessage(w, reply, false)
		trailers(w, 0, "")
		return
	case "twice":
		writeMessage(w, s.message("HelloReply"), false)
		writeMessage(w, s.message("HelloReply"), false)
		trailers(w, 0, "")
		return
	case "slow":
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	reply := s.message("HelloReply")
	rf := reply.Descriptor().Fields()
	reply.Set(rf.ByName("message"), protoreflect.ValueOfString("Hello "+name))
	reply.Set(rf.ByName("times"), req.Get(fields.ByName("times")))
	reply.Set(rf.ByName("mood"), protoreflect.ValueOfEnum(1))
	reply.Set(rf.ByName("big"), protoreflect.ValueOfInt64(1<<60))
	extra, _ := anypb.New(req)
	extraMsg := dynamicpb.NewMessage(rf.ByName("extra").Message())
	b, _ := proto.Marshal(extra)
	_ = proto.Unmarshal(b, extraMsg)
	reply.Set(rf.ByName("extra"), protoreflect.ValueOfMessage(extraMsg))
	gz := name == "gzip"
	if gz {
		w.Header().Set("Grpc-Encoding", "gzip")
	}
	writeMessage(w, reply, gz)
	trailers(w, 0, "")
}

// reflect answers one reflection request: file_containing_symbol (field
// 4) with the file alone, file_by_filename (field 3) with the named file.
func (s *grpcTestServer) reflect(w http.ResponseWriter, msg []byte, version string) {
	if s.reflection != version {
		trailersOnly(w, 12, "unknown service")
		return
	}
	num, _, n := protowire.ConsumeTag(msg)
	arg, _ := protowire.ConsumeString(msg[n:])
	var fd protoreflect.FileDescriptor
	switch {
	case num == 4 && arg == "sonde.test.Greeter":
		fd = s.files
	case num == 3:
		fd = findFile(s.files, arg)
	}
	var reply []byte
	if fd == nil {
		var e []byte
		e = protowire.AppendTag(e, 1, protowire.VarintType)
		e = protowire.AppendVarint(e, 5)
		e = protowire.AppendTag(e, 2, protowire.BytesType)
		e = protowire.AppendString(e, "not found: "+arg)
		reply = protowire.AppendTag(reply, 7, protowire.BytesType)
		reply = protowire.AppendBytes(reply, e)
	} else {
		b, _ := proto.Marshal(protodesc.ToFileDescriptorProto(fd))
		var fdr []byte
		fdr = protowire.AppendTag(fdr, 1, protowire.BytesType)
		fdr = protowire.AppendBytes(fdr, b)
		reply = protowire.AppendTag(reply, 4, protowire.BytesType)
		reply = protowire.AppendBytes(reply, fdr)
	}
	writeFrame(w, reply, false)
	trailers(w, 0, "")
}

// findFile finds an imported file by path, including well-known types.
func findFile(fd protoreflect.FileDescriptor, path string) protoreflect.FileDescriptor {
	if fd.Path() == path {
		return fd
	}
	imports := fd.Imports()
	for i := range imports.Len() {
		if f := findFile(imports.Get(i).FileDescriptor, path); f != nil {
			return f
		}
	}
	if f, err := protoregistry.GlobalFiles.FindFileByPath(path); err == nil {
		return f
	}
	return nil
}

func writeMessage(w io.Writer, m proto.Message, gz bool) {
	b, _ := proto.Marshal(m)
	writeFrame(w, b, gz)
}

func writeFrame(w io.Writer, b []byte, gz bool) {
	flag := byte(0)
	if gz {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(b)
		_ = zw.Close()
		b, flag = buf.Bytes(), 1
	}
	head := make([]byte, 5)
	head[0] = flag
	binary.BigEndian.PutUint32(head[1:], uint32(len(b))) //nolint:gosec // test
	_, _ = w.Write(append(head, b...))
}

// trailers ends a call with a status in the trailers.
func trailers(w http.ResponseWriter, code int, msg string) {
	w.Header().Set(http.TrailerPrefix+"Grpc-Status", strconv.Itoa(code))
	if msg != "" {
		w.Header().Set(http.TrailerPrefix+"Grpc-Message", url.PathEscape(msg))
	}
}

// trailersOnly ends a call without messages: the status is in the headers.
func trailersOnly(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Grpc-Status", strconv.Itoa(code))
	w.Header().Set("Grpc-Message", strings.ReplaceAll(url.PathEscape(msg), "%20", " "))
	w.WriteHeader(http.StatusOK)
}
