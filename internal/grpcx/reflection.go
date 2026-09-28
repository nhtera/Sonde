// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// reflectionVersions are the server reflection services tried, in order.
var reflectionVersions = []string{"grpc.reflection.v1", "grpc.reflection.v1alpha"}

// Invoke sends one request message to the method path of the server and
// returns the reply message. A call that ends with another status than OK
// returns a *StatusError.
type Invoke func(ctx context.Context, path string, request []byte) ([]byte, error)

// Reflect loads the descriptors of service from the server's reflection
// service: grpc.reflection.v1, or v1alpha when the server does not
// implement v1. Each question is one call.
func Reflect(ctx context.Context, service string, invoke Invoke) (*Descriptors, error) {
	var errs []string
	for _, v := range reflectionVersions {
		path := "/" + v + ".ServerReflection/ServerReflectionInfo"
		d, err := reflectWith(ctx, service, path, invoke)
		if err == nil {
			return d, nil
		}
		var se *StatusError
		if !errors.As(err, &se) || se.Status.Code != CodeUnimplemented {
			return nil, fmt.Errorf("server reflection (%s): %w", v, err)
		}
		errs = append(errs, v)
	}
	return nil, fmt.Errorf("the server implements no reflection service (tried %s): name a proto or protoset file in [SondeGrpc]", strings.Join(errs, ", "))
}

// maxReflectedFiles bounds the files one reflection asks for.
const maxReflectedFiles = 1000

// Fields of the reflection messages (grpc/reflection/v1/reflection.proto,
// identical in v1alpha).
const (
	reqFileByFilename       = 3
	reqFileContainingSymbol = 4
	respFileDescriptor      = 4
	respError               = 7
	fileDescriptorProto     = 1
	errorCode               = 1
	errorMessage            = 2
)

// reflectWith asks for the file defining service, then for every import
// the replies do not include yet.
func reflectWith(ctx context.Context, service, path string, invoke Invoke) (*Descriptors, error) {
	files := map[string]*descriptorpb.FileDescriptorProto{}
	var order []*descriptorpb.FileDescriptorProto
	ask := func(field protowire.Number, value string) error {
		req := protowire.AppendTag(nil, field, protowire.BytesType)
		req = protowire.AppendString(req, value)
		reply, err := invoke(ctx, path, req)
		if err != nil {
			return err
		}
		fds, err := parseReflectionReply(reply)
		if err != nil {
			return err
		}
		for _, fd := range fds {
			if files[fd.GetName()] == nil {
				files[fd.GetName()] = fd
				order = append(order, fd)
			}
		}
		return nil
	}
	if err := ask(reqFileContainingSymbol, service); err != nil {
		return nil, err
	}
	for i := 0; i < len(order); i++ {
		if len(order) > maxReflectedFiles {
			return nil, fmt.Errorf("the server returned more than %d files", maxReflectedFiles)
		}
		for _, dep := range order[i].GetDependency() {
			if files[dep] != nil {
				continue
			}
			if err := ask(reqFileByFilename, dep); err != nil {
				return nil, err
			}
			if files[dep] == nil {
				return nil, fmt.Errorf("the server did not return the file %s", dep)
			}
		}
	}
	return newDescriptors(order)
}

// parseReflectionReply reads the files of a ServerReflectionResponse, or
// its error.
func parseReflectionReply(b []byte) ([]*descriptorpb.FileDescriptorProto, error) {
	var files []*descriptorpb.FileDescriptorProto
	err := eachField(b, func(num protowire.Number, v []byte) error {
		switch num {
		case respFileDescriptor:
			return eachField(v, func(num protowire.Number, v []byte) error {
				if num != fileDescriptorProto {
					return nil
				}
				fd := &descriptorpb.FileDescriptorProto{}
				if err := proto.Unmarshal(v, fd); err != nil {
					return fmt.Errorf("invalid file descriptor: %w", err)
				}
				files = append(files, fd)
				return nil
			})
		case respError:
			msg := ""
			_ = eachField(v, func(num protowire.Number, v []byte) error {
				if num == errorMessage {
					msg = string(v)
				}
				return nil
			})
			return &StatusError{Status: NewStatus(errorCodeOf(v), msg)}
		}
		return nil
	})
	return files, err
}

// errorCodeOf reads the error_code varint of an ErrorResponse.
func errorCodeOf(b []byte) int {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return CodeUnknown
		}
		b = b[n:]
		if num == errorCode && typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return CodeUnknown
			}
			return int(int32(v)) //nolint:gosec // G115: an int32 field
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return CodeUnknown
		}
		b = b[n:]
	}
	return CodeUnknown
}

// eachField calls f with each length-delimited field of a message; other
// fields are skipped.
func eachField(b []byte, f func(protowire.Number, []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return errors.New("invalid reflection reply")
		}
		b = b[n:]
		if typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return errors.New("invalid reflection reply")
			}
			if err := f(num, v); err != nil {
				return err
			}
			b = b[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return errors.New("invalid reflection reply")
		}
		b = b[n:]
	}
	return nil
}
