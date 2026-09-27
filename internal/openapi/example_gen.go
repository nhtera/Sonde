// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"math"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// maxExampleDepth bounds nested and recursive schemas, maxExampleNodes the
// size of an example (wide recursive schemas grow exponentially).
const (
	maxExampleDepth = 8
	maxExampleNodes = 1000
	// maxExampleItems and maxExampleString cap minItems and minLength.
	maxExampleItems  = 100
	maxExampleString = 4096
)

// exampler builds one example within the node budget.
type exampler struct {
	nodes int
	// response builds a response body: writeOnly properties are left out
	// instead of readOnly ones.
	response bool
}

// Example returns a deterministic instance of the schema: its example
// (or first `examples` entry, const, default or first enum value), else a
// minimal instance built from its type: required properties only, one
// array item (or minItems), the first oneOf/anyOf branch, bounds and
// string formats honored.
func Example(ref *openapi3.SchemaRef) any {
	return (&exampler{}).example(ref, 0)
}

// ResponseExample is Example for a response body: readOnly properties
// are kept and writeOnly ones left out.
func ResponseExample(ref *openapi3.SchemaRef) any {
	return (&exampler{response: true}).example(ref, 0)
}

func (x *exampler) example(ref *openapi3.SchemaRef, depth int) any {
	x.nodes++
	if ref == nil || ref.Value == nil || depth > maxExampleDepth || x.nodes > maxExampleNodes {
		return nil
	}
	s := ref.Value
	switch {
	case s.Example != nil:
		return s.Example
	case len(s.Examples) > 0:
		return s.Examples[0]
	case s.Const != nil:
		return s.Const
	case s.Default != nil:
		return s.Default
	case len(s.Enum) > 0:
		return s.Enum[0]
	case len(s.AllOf) > 0:
		return x.allOfExample(s, depth)
	case len(s.OneOf) > 0:
		return x.example(s.OneOf[0], depth+1)
	case len(s.AnyOf) > 0:
		return x.example(s.AnyOf[0], depth+1)
	}
	switch schemaType(s) {
	case openapi3.TypeObject:
		return x.objectExample(s, depth)
	case openapi3.TypeArray:
		return x.arrayExample(s, depth)
	case openapi3.TypeString:
		return stringExample(s)
	case openapi3.TypeInteger:
		return int64(math.Round(numberExample(s, 1)))
	case openapi3.TypeNumber:
		return numberExample(s, 0)
	case openapi3.TypeBoolean:
		return true
	case openapi3.TypeNull:
		return nil
	}
	return nil
}

// schemaType is the type of s: its first non-null type, else implied by
// its keywords.
func schemaType(s *openapi3.Schema) string {
	if s.Type != nil {
		for _, t := range s.Type.Slice() {
			if t != openapi3.TypeNull {
				return t
			}
		}
		if s.Type.IncludesNull() {
			return openapi3.TypeNull
		}
	}
	switch {
	case len(s.Properties) > 0 || len(s.Required) > 0:
		return openapi3.TypeObject
	case s.Items != nil:
		return openapi3.TypeArray
	}
	return ""
}

func (x *exampler) objectExample(s *openapi3.Schema, depth int) any {
	obj := map[string]any{}
	names := append([]string(nil), s.Required...)
	sort.Strings(names)
	for _, name := range names {
		if p, ok := s.Properties[name]; ok {
			if p.Value != nil && ((!x.response && p.Value.ReadOnly) || (x.response && p.Value.WriteOnly)) {
				continue
			}
			obj[name] = x.example(p, depth+1)
		} else {
			obj[name] = ""
		}
	}
	return obj
}

func (x *exampler) allOfExample(s *openapi3.Schema, depth int) any {
	merged := map[string]any{}
	var other any
	for _, sub := range s.AllOf {
		switch v := x.example(sub, depth+1).(type) {
		case map[string]any:
			for k, x := range v {
				merged[k] = x
			}
		default:
			other = v
		}
	}
	if len(s.Properties) > 0 || len(s.Required) > 0 {
		if o, ok := x.objectExample(s, depth).(map[string]any); ok {
			for k, x := range o {
				merged[k] = x
			}
		}
	}
	if len(merged) == 0 && other != nil {
		return other
	}
	return merged
}

func (x *exampler) arrayExample(s *openapi3.Schema, depth int) any {
	n := min(max(s.MinItems, 1), maxExampleItems)
	if s.MaxItems != nil && *s.MaxItems < n {
		n = *s.MaxItems
	}
	items := make([]any, 0, n)
	for i := uint64(0); i < n; i++ {
		var ref *openapi3.SchemaRef
		if i < uint64(len(s.PrefixItems)) {
			ref = s.PrefixItems[i]
		} else {
			ref = s.Items
		}
		items = append(items, x.example(ref, depth+1))
	}
	return items
}

// formatExamples are values of the common string formats.
var formatExamples = map[string]string{
	"email":         "user@example.com",
	"idn-email":     "user@example.com",
	"uuid":          "00000000-0000-4000-8000-000000000000",
	"date-time":     "2026-01-01T00:00:00Z",
	"date":          "2026-01-01",
	"time":          "00:00:00Z",
	"uri":           "https://example.com",
	"url":           "https://example.com",
	"iri":           "https://example.com",
	"uri-reference": "/",
	"hostname":      "example.com",
	"idn-hostname":  "example.com",
	"ipv4":          "192.0.2.1",
	"ipv6":          "2001:db8::1",
	"byte":          "c29uZGU=",
	"password":      "password",
	"duration":      "PT1S",
}

func stringExample(s *openapi3.Schema) string {
	v, ok := formatExamples[s.Format]
	if !ok {
		v = "string"
	}
	if n := min(s.MinLength, maxExampleString); uint64(len(v)) < n {
		v += strings.Repeat("x", int(n)-len(v)) //nolint:gosec // G115: n <= maxExampleString
	}
	if s.MaxLength != nil && uint64(len(v)) > *s.MaxLength {
		v = v[:*s.MaxLength]
	}
	return v
}

// numberExample is a value within the bounds of s, near def.
func numberExample(s *openapi3.Schema, def float64) float64 {
	v := def
	lo, hi := s.Min, s.Max
	if s.ExclusiveMin.Value != nil {
		lo = s.ExclusiveMin.Value
	}
	if s.ExclusiveMax.Value != nil {
		hi = s.ExclusiveMax.Value
	}
	loExcl := s.ExclusiveMin.Value != nil || s.ExclusiveMin.IsTrue()
	hiExcl := s.ExclusiveMax.Value != nil || s.ExclusiveMax.IsTrue()
	if lo != nil && (v < *lo || (loExcl && v <= *lo)) {
		v = *lo
		if loExcl {
			v++
		}
	}
	if hi != nil && (v > *hi || (hiExcl && v >= *hi)) {
		v = *hi
		if hiExcl {
			v--
		}
	}
	return v
}
