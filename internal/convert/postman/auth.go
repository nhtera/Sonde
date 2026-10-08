// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// auth is a Postman auth object: its params are grouped by type, one array
// of {key, value} per type actually used (schema.postman.com allows any of
// them regardless of Type, but a real export only ever fills the one named
// by Type).
type auth struct {
	Type   string        `json:"type"`
	Basic  authParamList `json:"basic"`
	Bearer authParamList `json:"bearer"`
	Apikey authParamList `json:"apikey"`
	Digest authParamList `json:"digest"`
	Awsv4  authParamList `json:"awsv4"`
	Ntlm   authParamList `json:"ntlm"`
	Oauth2 authParamList `json:"oauth2"`
}

type authParam struct {
	Key   string     `json:"key"`
	Value flexString `json:"value"`
}

// authParamList accepts the usual [{key, value}, ...] array, and the
// {"username": "u", ...} object shape a v2.0 export sometimes writes for
// the same params; its keys are sorted for a deterministic result.
type authParamList []authParam

func (l *authParamList) UnmarshalJSON(b []byte) error {
	var arr []authParam
	if json.Unmarshal(b, &arr) == nil {
		*l = arr
		return nil
	}
	var obj map[string]flexString
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]authParam, len(keys))
	for i, k := range keys {
		out[i] = authParam{Key: k, Value: obj[k]}
	}
	*l = out
	return nil
}

func paramValue(params []authParam, key string) string {
	for _, p := range params {
		if p.Key == key {
			return string(p.Value)
		}
	}
	return ""
}

// authState is the auth in effect at one point of the item tree: the
// nearest enclosing auth field, or none.
type authState struct{ auth *auth }

// resolve returns the auth in effect for a node whose own auth field is
// own: own, when set (including "noauth", which then means no auth and
// stops inheriting s, exactly like any other explicit auth), otherwise s
// unchanged. Only a request or a folder's own auth field can appear here;
// an absent field inherits.
func (s authState) resolve(own *auth) authState {
	if own == nil {
		return s
	}
	return authState{auth: own}
}

// awsRegionService formats the region/service half of an aws-sigv4 option
// value ("aws:amz:REGION:SERVICE"); either may be empty.
func awsRegionService(region, service string) string {
	return "aws:amz:" + region + ":" + service
}

// applyAuth adds the effective auth of st to e: [BasicAuth] for basic, an
// Authorization header for bearer, a header or query parameter for apikey.
// digest, ntlm and awsv4 get the user option and the matching [Options]
// flag ([BasicAuth] is a literal Basic header, which no scheme answers a
// challenge with or signs). Any other type (oauth1, oauth2, hawk,
// edgegrid, jwt, asap, or an unrecognized one) has no Sonde equivalent and
// is warned about by name.
func (w *walker) applyAuth(e *syntax.EntrySpec, st authState, name string) {
	a := st.auth
	if a == nil || a.Type == "" || a.Type == "noauth" {
		return
	}
	switch a.Type {
	case "basic":
		e.BasicAuth = w.basicAuth(paramValue(a.Basic, "username"), paramValue(a.Basic, "password"))
	case "bearer":
		tt, ws := convert.ParseText(paramValue(a.Bearer, "token"))
		w.addWarnings(ws)
		e.Headers = append(e.Headers, syntax.Field{
			Key:   syntax.PlainText("Authorization"),
			Value: append(syntax.Text{syntax.Lit("Bearer ")}, tt...),
		})
	case "apikey":
		kt, kw := convert.ParseText(paramValue(a.Apikey, "key"))
		vt, vw := convert.ParseText(paramValue(a.Apikey, "value"))
		w.addWarnings(kw)
		w.addWarnings(vw)
		f := syntax.Field{Key: kt, Value: vt}
		if strings.EqualFold(paramValue(a.Apikey, "in"), "query") {
			e.Query = append(e.Query, f)
		} else {
			e.Headers = append(e.Headers, f)
		}
	case "digest":
		e.Options = append(e.Options, w.userOption(paramValue(a.Digest, "username"), paramValue(a.Digest, "password")),
			syntax.BoolOption("digest", true))
	case "ntlm":
		e.Options = append(e.Options, w.userOption(paramValue(a.Ntlm, "username"), paramValue(a.Ntlm, "password")),
			syntax.BoolOption("ntlm", true))
	case "awsv4":
		sigv4 := awsRegionService(paramValue(a.Awsv4, "region"), paramValue(a.Awsv4, "service"))
		e.Options = append(e.Options, w.userOption(paramValue(a.Awsv4, "accessKey"), paramValue(a.Awsv4, "secretKey")),
			syntax.StringOption("aws-sigv4", syntax.PlainText(sigv4)))
		if tok := paramValue(a.Awsv4, "sessionToken"); tok != "" {
			tt, ws := convert.ParseText(tok)
			w.addWarnings(ws)
			e.Headers = append(e.Headers, syntax.Field{Key: syntax.PlainText("X-Amz-Security-Token"), Value: tt})
		}
	default:
		w.warn(convert.WarnUnsupportedAuth, fmt.Sprintf("%s: auth type %q has no Sonde equivalent; add it by hand", name, a.Type))
	}
}

// userOption is the user option "user:pass" of a scheme other than Basic.
func (w *walker) userOption(user, pass string) syntax.OptionField {
	ut, uw := convert.ParseText(user)
	pt, pw := convert.ParseText(pass)
	w.addWarnings(uw)
	w.addWarnings(pw)
	v := append(append(append(syntax.Text{}, ut...), syntax.Lit(":")), pt...)
	return syntax.StringOption("user", v)
}

func (w *walker) basicAuth(user, pass string) *syntax.BasicAuth {
	ut, uw := convert.ParseText(user)
	pt, pw := convert.ParseText(pass)
	w.addWarnings(uw)
	w.addWarnings(pw)
	return &syntax.BasicAuth{User: ut, Password: pt}
}

// scriptCode is the joined source of a Postman script's "exec", which a
// real export writes as an array of lines, tolerating the single-string
// shape too; "" means no script.
func scriptCode(s *script) string {
	if s == nil || len(s.Exec) == 0 {
		return ""
	}
	var lines []string
	if json.Unmarshal(s.Exec, &lines) == nil {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	var one string
	if json.Unmarshal(s.Exec, &one) == nil {
		return strings.TrimSpace(one)
	}
	return ""
}

func eventPhase(e event) string {
	if e.Listen == "" {
		return "script"
	}
	return e.Listen
}

// scriptComments renders every non-empty, non-disabled script of events as
// a "# ..." comment block, in order; scripts are never executed.
func scriptComments(events []event) []string {
	var out []string
	for _, ev := range events {
		if ev.Disabled {
			continue
		}
		if code := scriptCode(ev.Script); code != "" {
			out = append(out, eventPhase(ev)+" script (never executed):\n"+code)
		}
	}
	return out
}

// warnEvents records one WarnScript per non-empty script of events, kept
// (as scriptComments, where the caller attaches them) but never executed.
func (w *walker) warnEvents(name string, events []event) {
	for _, ev := range events {
		if ev.Disabled || scriptCode(ev.Script) == "" {
			continue
		}
		w.warn(convert.WarnScript, fmt.Sprintf("%s: %s script kept as a comment, not executed", name, eventPhase(ev)))
	}
}

// statusHave matches pm.response.to.have.status(N[, "reason"]); statusEql
// matches pm.expect(pm.response.code).to.eql(N) or .to.equal(N). Only these
// trivially safe status-code checks are translated to an expected response;
// every other assertion stays inert, in the comment.
var (
	statusHave = regexp.MustCompile(`pm\.response\.to\.have\.status\(\s*(\d{3})\s*[,)]`)
	statusEql  = regexp.MustCompile(`pm\.expect\(pm\.response\.code\)\.to\.(?:eql|equal)\(\s*(\d{3})\s*\)`)
)

func extractStatus(code string) string {
	if m := statusHave.FindStringSubmatch(code); m != nil {
		return m[1]
	}
	if m := statusEql.FindStringSubmatch(code); m != nil {
		return m[1]
	}
	return ""
}

// applyEvents adds every non-disabled event of it as a comment on e (its
// script is never executed) and, for a "test" script, translates a
// recognized status-code assertion to e.Response.
func (w *walker) applyEvents(e *syntax.EntrySpec, events []event, name string) {
	w.warnEvents(name, events)
	for _, ev := range events {
		if ev.Disabled {
			continue
		}
		code := scriptCode(ev.Script)
		if code == "" {
			continue
		}
		e.Comments = append(e.Comments, eventPhase(ev)+" script (never executed):\n"+code)
		if ev.Listen == "test" && e.Response == nil {
			if status := extractStatus(code); status != "" {
				e.Response = &syntax.ResponseSpec{Status: status}
			}
		}
	}
}
