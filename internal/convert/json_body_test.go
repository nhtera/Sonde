// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

func TestJSONBody(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{`{"z": 1, "a": {"b": [1.50, true, null]}, "z": "{{v}}"}`, `{"z": 1, "a": {"b": [1.50, true, null]}, "z": "{{v}}"}`, true},
		{`  [ "{{$uuid}}", -0.0e1 ]  `, `["{{newUuid}}", -0.0e1]`, true},
		{`"str"`, `"str"`, true},
		{`{"{{field}}": "v"}`, ``, false},
		{`{"n": {{n}}}`, ``, false},
		{`{"a": 1} {"b": 2}`, ``, false},
		{`{"a": 1`, ``, false},
		{strings.Repeat("[", 600) + strings.Repeat("]", 600), ``, false},
		// The parser's limit: 128 levels.
		{strings.Repeat("[", 128) + "1" + strings.Repeat("]", 128), strings.Repeat("[", 128) + "1" + strings.Repeat("]", 128), true},
		{strings.Repeat("[", 129) + "1" + strings.Repeat("]", 129), ``, false},
	}
	for _, tt := range tests {
		body, _, ok := JSONBody(tt.in)
		if ok != tt.ok {
			t.Errorf("JSONBody(%.40q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		f, err := syntax.BuildFile([]syntax.EntrySpec{{Method: "POST", URL: syntax.PlainText("http://x/"), Body: body}}, syntax.DialectHurl)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSuffix(strings.TrimPrefix(string(syntax.Format(f)), "POST http://x/\n"), "\n")
		if got != tt.want {
			t.Errorf("JSONBody(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
	if _, warns, _ := JSONBody(`{"t": "{{$timestamp}}"}`); len(warns) != 1 || warns[0].Kind != WarnDynamicVariable {
		t.Errorf("warnings = %v", warns)
	}
}
