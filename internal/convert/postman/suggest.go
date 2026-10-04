// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/convert/suggest"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

// Suggestions are edits an import proposes on top of its files, for a
// user to accept one by one: asserts and captures read from the test
// scripts of the collection (never executed, package suggest), and a
// login entry for OAuth2.

// Suggest returns the suggestions of the import of data under opts: the
// same collection walk as Import, whose Output.Files the suggestions'
// File indices address.
func Suggest(data []byte, opts Options) (out []suggest.FileSuggestion, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, fmt.Errorf("postman: malformed collection: %v", p)
		}
	}()
	w, err := walk(data, opts)
	if err != nil {
		return nil, err
	}
	return w.suggestions, nil
}

// scripts reads the test scripts of a collection.
var scripts = suggest.NewReader(suggest.API{
	Test:      `pm\.test`,
	Expect:    `pm\.expect`,
	Body:      `pm\.response\.json\(\)`,
	Status:    `pm\.response\.code`,
	HeaderGet: `pm\.response\.headers\.get`,
	HeaderHas: `pm\.response\.to\.have\.header`,
	Set:       `pm\.(?:environment|collectionVariables|globals|variables)\.set`,
})

// entryOps are the suggestions of one entry, before its file is known.
type entryOps struct {
	suggest.Entry
	oauth2 *syntaxedit.LoginSpec
}

// fileSuggestions turns the ops of each entry (1-based) of file into
// suggestions, one per kind.
func (w *walker) fileSuggestions(file int, entries []entryOps) {
	var read []suggest.Entry
	var login []suggest.Op
	for i, e := range entries {
		read = append(read, e.Entry)
		if e.oauth2 != nil {
			n := i + 1
			if len(login) == 0 {
				login = append(login, suggest.Op{Entry: n, Login: e.oauth2})
			}
			login = append(login, suggest.Op{Entry: n, Section: syntaxedit.Headers, Key: "Authorization", Value: "Bearer {{" + e.oauth2.Capture + "}}"})
		}
	}
	w.suggestions = append(w.suggestions, suggest.Suggestions(file, read)...)
	if len(login) > 0 {
		w.suggestions = append(w.suggestions, suggest.FileSuggestion{File: file, Label: "Log in with OAuth2 and send the token", Ops: login})
	}
}

// entryOps reads the suggestions of a request item: its test scripts,
// and its OAuth2 auth.
func (w *walker) entryOps(it item, a authState) entryOps {
	var ops entryOps
	ops.oauth2 = oauth2Login(a.auth)
	for _, ev := range it.Event {
		if ev.Disabled || ev.Listen != "test" {
			continue
		}
		asserts, captures := scripts.Read(scriptCode(ev.Script))
		ops.Asserts = append(ops.Asserts, asserts...)
		ops.Captures = append(ops.Captures, captures...)
	}
	return ops
}

// oauth2Login reads an oauth2 auth as a login entry capturing
// access_token, for the grant types that are one form post.
func oauth2Login(a *auth) *syntaxedit.LoginSpec {
	if a == nil || a.Type != "oauth2" {
		return nil
	}
	tokenURL := paramValue(a.Oauth2, "accessTokenUrl")
	grant := paramValue(a.Oauth2, "grant_type")
	if tokenURL == "" {
		return nil
	}
	fields := map[string]string{
		"client_credentials": "client_id=clientId client_secret=clientSecret scope=scope",
		"password":           "client_id=clientId client_secret=clientSecret username=username password=password scope=scope",
	}[grant]
	if fields == "" {
		return nil
	}
	urlText, _ := convert.ParseText(tokenURL)
	form := []syntax.Field{syntax.KV("grant_type", grant)}
	for _, f := range strings.Fields(fields) {
		key, param, _ := strings.Cut(f, "=")
		if v := paramValue(a.Oauth2, param); v != "" {
			vt, _ := convert.ParseText(v)
			form = append(form, syntax.Field{Key: syntax.PlainText(key), Value: vt})
		}
	}
	return &syntaxedit.LoginSpec{
		Request:  syntax.EntrySpec{Comments: []string{"OAuth2 " + grant + " login"}, Method: "POST", URL: urlText, Form: form},
		Capture:  "access_token",
		JSONPath: "$.access_token",
		Redact:   true,
	}
}
