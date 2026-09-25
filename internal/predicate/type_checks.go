// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package predicate

import (
	"bytes"
	"net/netip"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/datefmt"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

func startsWith(actual, expected value.Value) (ok, typed bool) {
	switch a := actual.(type) {
	case value.String:
		if e, typed := expected.(value.String); typed {
			return strings.HasPrefix(string(a), string(e)), true
		}
	case value.Bytes:
		if e, typed := expected.(value.Bytes); typed {
			return bytes.HasPrefix(a, e), true
		}
	}
	return false, false
}

func endsWith(actual, expected value.Value) (ok, typed bool) {
	switch a := actual.(type) {
	case value.String:
		if e, typed := expected.(value.String); typed {
			return strings.HasSuffix(string(a), string(e)), true
		}
	case value.Bytes:
		if e, typed := expected.(value.Bytes); typed {
			return bytes.HasSuffix(a, e), true
		}
	}
	return false, false
}

func contains(actual, expected value.Value) (ok, typed bool) {
	switch a := actual.(type) {
	case value.String:
		if e, typed := expected.(value.String); typed {
			return strings.Contains(string(a), string(e)), true
		}
	case value.Bytes:
		if e, typed := expected.(value.Bytes); typed {
			return bytes.Contains(a, e), true
		}
	case value.List:
		return includes(a, expected), true
	}
	return false, false
}

func includes(list value.List, v value.Value) bool {
	for _, e := range list {
		if value.Equal(e, v) {
			return true
		}
	}
	return false
}

// count is the size used by isEmpty; a string counts its bytes.
func count(v value.Value) (int, bool) {
	switch v := v.(type) {
	case value.List:
		return len(v), true
	case value.String:
		return len(v), true
	case value.Nodeset:
		return int(v), true
	case value.Object:
		return len(v), true
	case value.Bytes:
		return len(v), true
	}
	return 0, false
}

func itoa(n int) string { return strconv.Itoa(n) }

// kindCheck evaluates the kind predicates; name is the expected kind.
func kindCheck(k syntax.PredicateKind, v value.Value) (name string, ok bool) {
	kind := v.Kind()
	switch k {
	case syntax.PredicateIsBoolean:
		return "boolean", kind == value.KindBool
	case syntax.PredicateIsCollection:
		return "collection", kind == value.KindBytes || kind == value.KindList || kind == value.KindNodeset || kind == value.KindObject
	case syntax.PredicateIsDate:
		return "date", kind == value.KindDate
	case syntax.PredicateIsFloat:
		return "float", kind == value.KindFloat
	case syntax.PredicateIsInteger:
		return "integer", kind == value.KindInteger
	case syntax.PredicateIsList:
		return "list", kind == value.KindBytes || kind == value.KindList
	case syntax.PredicateIsNumber:
		return "number", kind == value.KindInteger || kind == value.KindFloat
	case syntax.PredicateIsObject:
		return "object", kind == value.KindNodeset || kind == value.KindObject
	case syntax.PredicateIsString:
		return "string", kind == value.KindString
	}
	return k.String(), false
}

func isISODate(s string) bool {
	_, err := datefmt.ParseRFC3339(s)
	return err == nil
}

func isIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

func isIPv6(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is6() && a.Zone() == ""
}

// isUUID accepts the simple (32 hex digits), hyphenated, braced and URN
// forms, in any case.
func isUUID(s string) bool {
	switch {
	case len(s) == 32:
		return isHexDigits(s)
	case len(s) == 38 && s[0] == '{' && s[37] == '}':
		s = s[1:37]
	case len(s) == 45 && strings.EqualFold(s[:9], "urn:uuid:"):
		s = s[9:]
	}
	if len(s) != 36 {
		return false
	}
	for _, i := range []int{8, 13, 18, 23} {
		if s[i] != '-' {
			return false
		}
	}
	return isHexDigits(s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:])
}

func isHexDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
