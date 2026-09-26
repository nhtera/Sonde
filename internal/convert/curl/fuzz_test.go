// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// FuzzCurlImport checks that Import never panics on arbitrary, possibly
// malformed input, whichever dialect it targets, and that whatever *syntax.File
// it does return actually parses (BuildFile's own contract).
func FuzzCurlImport(f *testing.F) {
	for _, seed := range []string{
		"",
		"curl",
		"curl https://example.com",
		"curl -X POST https://example.com -d '{}'",
		"curl -H 'X:Y' $'https://example.com/\\x41'",
		"curl 'unterminated",
		"curl \"unterminated",
		"curl $'unterminated",
		"curl \\",
		"# comment only",
		"curl a; curl b && curl c",
		"curl -F 'x=@f;type=y' -F 'z=1' http://a",
		"curl -G -d 'a=1' --data-urlencode '@file' http://a",
		"curl --json '{}' -X GET http://a",
		"-not-curl-at-all",
		"curl " + string([]byte{0, 1, 2, 0xff, 0xfe}),
		`curl -H "Authorization: Bearer $TOKEN" https://a.com/$PATH`,
		`curl "${NAME}" "${" "${}" "${:-}" "${1abc}"`,
		`curl $ $$ $? $@ $# $* $- $12345678901234567890`,
		"curl \"$(nested $(cmd))\" `un`terminated`",
		`curl --data "a=$A&b=$(cmd)&c=` + "`x`" + `" http://a`,
		`curl \$ESCAPED "\$ALSO_ESCAPED" '$LITERAL'`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		for _, d := range []syntax.Dialect{syntax.DialectHurl, syntax.DialectSonde} {
			res, err := Import([]byte(src), d)
			if err != nil {
				continue
			}
			if res.File == nil {
				t.Fatalf("Import returned no error and no file for %q", src)
			}
			if _, perr := syntax.Parse("<fuzz>", syntax.Format(res.File), d); perr != nil {
				t.Fatalf("Import(%q) produced a file that does not reparse: %v", src, perr)
			}
		}
	})
}
