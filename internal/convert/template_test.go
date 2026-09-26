// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// render builds a one-entry file whose URL is t and returns its URL line.
func render(t *testing.T, text syntax.Text) string {
	t.Helper()
	f, err := syntax.BuildFile([]syntax.EntrySpec{{Method: "GET", URL: text}}, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := strings.Cut(string(syntax.Format(f)), "\n")
	return strings.TrimPrefix(line, "GET ")
}

func TestParseText(t *testing.T) {
	tests := []struct {
		in, want string
		warns    int
	}{
		{"http://x/{{host}}/a", "http://x/{{host}}/a", 0},
		{"{{ base url }}/p?id={{api.id}}", "{{base_url}}/p?id={{api_id}}", 0},
		{"/{{$guid}}/{{$uuid}}/{{$isoTimestamp}}", "/{{newUuid}}/{{newUuid}}/{{newDate}}", 0},
		{"/{{$timestamp}}", "/{{timestamp}}", 1},
		{"/{{$randomInt}}{{$datetime iso8601}}", "/{{randomInt}}{{datetime_iso8601}}", 2},
		{"/{{newDate}}", "/{{var_newDate}}", 0},
		{"/{{}}/{{unclosed", "/\\u{7B}{}}/\\u{7B}{unclosed", 0},
		{"plain", "plain", 0},
		{"{{{token}}}", "\\u{7B}{{token}}}", 0},
		{"{{a{{b}}}}", "\\u{7B}{a{{b}}}}", 0},
	}
	for _, tt := range tests {
		text, warns := ParseText(tt.in)
		if got := render(t, text); got != tt.want {
			t.Errorf("ParseText(%q) renders %q, want %q", tt.in, got, tt.want)
		}
		if len(warns) != tt.warns {
			t.Errorf("ParseText(%q) warnings = %v, want %d", tt.in, warns, tt.warns)
		}
		for _, w := range warns {
			if w.Kind != WarnDynamicVariable {
				t.Errorf("warning kind = %q", w.Kind)
			}
		}
	}
}

func TestVariableName(t *testing.T) {
	for in, want := range map[string]string{
		"host": "host", "api.host": "api_host", "a b": "a_b", "x-y_z9": "x-y_z9",
		"": "_", "newUuid": "var_newUuid", "héllo": "h_llo",
	} {
		if got := VariableName(in); got != want {
			t.Errorf("VariableName(%q) = %q, want %q", in, got, want)
		}
	}
}
