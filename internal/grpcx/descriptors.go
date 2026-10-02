// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Descriptors are the message types known to a call.
type Descriptors struct {
	files *protoregistry.Files
	types *dynamicpb.Types
}

// Method is the method of a call path "/package.Service/Method".
type Method struct {
	Path string
	desc protoreflect.MethodDescriptor
}

// ServerStreaming reports whether the server replies with a stream.
func (m *Method) ServerStreaming() bool { return m.desc.IsStreamingServer() }

// ClientStreaming reports whether the client sends a stream.
func (m *Method) ClientStreaming() bool { return m.desc.IsStreamingClient() }

// SplitPath splits a call path "/package.Service/Method" into its service
// and method names.
func SplitPath(path string) (service, method string, err error) {
	rest, ok := strings.CutPrefix(path, "/")
	if ok {
		service, method, ok = strings.Cut(rest, "/")
	}
	if !ok || service == "" || method == "" || strings.Contains(method, "/") {
		return "", "", fmt.Errorf("the URL path %q is not /package.Service/Method", path)
	}
	return service, method, nil
}

// Method finds the method of a call path.
func (d *Descriptors) Method(path string) (*Method, error) {
	service, method, err := SplitPath(path)
	if err != nil {
		return nil, err
	}
	desc, err := d.files.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil, fmt.Errorf("service %s is not in the descriptors", service)
	}
	sd, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a service", service)
	}
	md := sd.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil, fmt.Errorf("service %s has no method %s", service, method)
	}
	return &Method{Path: path, desc: md}, nil
}

// Files are the descriptor files of a call: .proto sources compiled at run
// time, with their import paths, and binary descriptor sets.
type Files struct {
	Protos      []string
	ImportPaths []string
	Protosets   []string
}

// Key identifies the descriptors Files load, for caching.
func (f Files) Key() string {
	return strings.Join(f.Protos, "\x00") + "\x01" + strings.Join(f.ImportPaths, "\x00") + "\x01" + strings.Join(f.Protosets, "\x00")
}

// Load reads and compiles the files through read, which confines them. A
// missing file must return an error wrapping fs.ErrNotExist, so that the
// next import path is tried.
func (f Files) Load(ctx context.Context, read func(name string) ([]byte, error)) (*Descriptors, error) {
	var set []*descriptorpb.FileDescriptorProto
	if len(f.Protos) > 0 {
		files, err := f.compile(ctx, read)
		if err != nil {
			return nil, err
		}
		set = append(set, files...)
	}
	for _, name := range f.Protosets {
		data, err := read(name)
		if err != nil {
			return nil, fmt.Errorf("could not read the descriptor set %s: %w", name, err)
		}
		var fds descriptorpb.FileDescriptorSet
		if err := proto.Unmarshal(data, &fds); err != nil {
			return nil, fmt.Errorf("%s is not a binary FileDescriptorSet: %w", name, err)
		}
		set = append(set, fds.File...)
	}
	return newDescriptors(set)
}

// compile compiles the .proto sources: each is named relative to the first
// import path that contains it, or to its own directory without any.
func (f Files) compile(ctx context.Context, read func(string) ([]byte, error)) ([]*descriptorpb.FileDescriptorProto, error) {
	imports := make([]string, len(f.ImportPaths))
	for i, p := range f.ImportPaths {
		imports[i] = filepath.Clean(p)
	}
	names := make([]string, len(f.Protos))
	for i, p := range f.Protos {
		p = filepath.Clean(p)
		if len(f.ImportPaths) == 0 {
			if dir := filepath.Dir(p); !slices.Contains(imports, dir) {
				imports = append(imports, dir)
			}
		}
		name, ok := "", false
		for _, dir := range imports {
			if rel, err := filepath.Rel(dir, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				name, ok = filepath.ToSlash(rel), true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("the proto file %s is in no import-path", f.Protos[i])
		}
		names[i] = name
	}
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: imports,
			Accessor: func(path string) (io.ReadCloser, error) {
				data, err := read(path)
				if err != nil {
					return nil, err
				}
				return io.NopCloser(bytes.NewReader(data)), nil
			},
		}),
		MaxParallelism: 1,
	}
	files, err := c.Compile(ctx, names...)
	if err != nil {
		return nil, fmt.Errorf("could not compile the proto files: %w", err)
	}
	var out []*descriptorpb.FileDescriptorProto
	seen := map[string]bool{}
	for _, file := range files {
		out = appendFile(out, seen, file)
	}
	return out, nil
}

// appendFile appends a compiled file after its imports, each once.
func appendFile(out []*descriptorpb.FileDescriptorProto, seen map[string]bool, fd protoreflect.FileDescriptor) []*descriptorpb.FileDescriptorProto {
	if seen[fd.Path()] {
		return out
	}
	seen[fd.Path()] = true
	imports := fd.Imports()
	for i := range imports.Len() {
		out = appendFile(out, seen, imports.Get(i).FileDescriptor)
	}
	return append(out, protodesc.ToFileDescriptorProto(fd))
}

// newDescriptors builds the descriptors of a set of files; a file named
// twice is kept once.
func newDescriptors(files []*descriptorpb.FileDescriptorProto) (*Descriptors, error) {
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	for _, f := range files {
		if !seen[f.GetName()] {
			seen[f.GetName()] = true
			set.File = append(set.File, f)
		}
	}
	reg, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("invalid descriptors: %w", err)
	}
	return &Descriptors{files: reg, types: dynamicpb.NewTypes(reg)}, nil
}

// Service is a service of the descriptors and its methods.
type Service struct {
	// Name is the full name, e.g. "package.Service".
	Name    string
	Methods []MethodInfo
}

// MethodInfo describes a method of a service.
type MethodInfo struct {
	Name string
	// Path is the call path "/package.Service/Method".
	Path            string
	ClientStreaming bool
	ServerStreaming bool
	// Input and Output are the full names of the message types.
	Input, Output string
	// InputFields are the input message's fields, in declaration order.
	InputFields []FieldInfo
}

// FieldInfo describes a field of a message: Type is as written in a
// .proto ("string", "repeated int32", "map<string, Item>", "pkg.Item").
type FieldInfo struct {
	Name   string
	Type   string
	Number int
}

// fieldsOf lists md's fields.
func fieldsOf(md protoreflect.MessageDescriptor) []FieldInfo {
	var out []FieldInfo
	for i := range md.Fields().Len() {
		fd := md.Fields().Get(i)
		out = append(out, FieldInfo{Name: string(fd.Name()), Type: typeName(fd), Number: int(fd.Number())})
	}
	return out
}

// typeName is fd's type as a .proto writes it.
func typeName(fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return "map<" + typeName(fd.MapKey()) + ", " + typeName(fd.MapValue()) + ">"
	}
	var t string
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		t = string(fd.Message().FullName())
	case protoreflect.EnumKind:
		t = string(fd.Enum().FullName())
	default:
		t = fd.Kind().String()
	}
	if fd.IsList() {
		return "repeated " + t
	}
	return t
}

// Services lists every service of the descriptors, sorted by name, with
// its methods in declaration order.
func (d *Descriptors) Services() []Service {
	var out []Service
	d.files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			s := Service{Name: string(sd.FullName())}
			for j := range sd.Methods().Len() {
				md := sd.Methods().Get(j)
				s.Methods = append(s.Methods, MethodInfo{
					Name:            string(md.Name()),
					Path:            "/" + s.Name + "/" + string(md.Name()),
					ClientStreaming: md.IsStreamingClient(),
					ServerStreaming: md.IsStreamingServer(),
					Input:           string(md.Input().FullName()),
					Output:          string(md.Output().FullName()),
					InputFields:     fieldsOf(md.Input()),
				})
			}
			out = append(out, s)
		}
		return true
	})
	slices.SortFunc(out, func(a, b Service) int { return strings.Compare(a.Name, b.Name) })
	return out
}
