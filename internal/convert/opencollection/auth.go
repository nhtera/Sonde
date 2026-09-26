// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// ancestorState is the inherited state a folder or the collection root
// passes down to its children: headers already merged and filtered, and
// the nearest non-inherit auth found so far (mapping doc, "Auth").
type ancestorState struct {
	headers []header
	auth    *auth
}

// extend folds one more level's defaults (the collection root's, or one
// folder's, "request" block) into anc, returning the state anc's children
// see.
func (anc ancestorState) extend(defs requestDefs) ancestorState {
	next := ancestorState{headers: mergeHeaders(anc.headers, defs.Headers), auth: anc.auth}
	if defs.Auth != nil && !defs.Auth.isInherit() {
		next.auth = defs.Auth
	}
	return next
}

// resolveAuth returns own if it is set and not "inherit", else the
// nearest ancestor auth (mapping doc, "Auth"); both may be nil. A
// resolved auth typed "none" (an explicit "no auth here", never look
// further up) resolves to no auth at all rather than reaching applyAuth,
// which would otherwise warn about an "unsupported" scheme.
func resolveAuth(anc *auth, own *auth) *auth {
	a := anc
	if own != nil && !own.isInherit() {
		a = own
	}
	if a != nil && a.Type == "none" {
		return nil
	}
	return a
}

// mergeHeaders flattens layers (root-first) into one list: a header
// disabled in any layer where it appears is dropped from that layer, and a
// name repeated in a later layer replaces the earlier value in place,
// keeping the position of its first occurrence.
func mergeHeaders(layers ...[]header) []header {
	var out []header
	pos := map[string]int{}
	for _, layer := range layers {
		for _, h := range layer {
			if h.Disabled {
				continue
			}
			if i, ok := pos[h.Name]; ok {
				out[i] = h
				continue
			}
			pos[h.Name] = len(out)
			out = append(out, h)
		}
	}
	return out
}

// applyAuth resolves a onto e: [BasicAuth], a bearer/apikey header or
// query field, or a WarnUnsupportedAuth for a scheme with no Sonde
// equivalent (mapping doc, "Auth"). a may be nil.
func applyAuth(name string, a *auth, e *syntax.EntrySpec) []convert.Warning {
	if a == nil || a.isInherit() {
		return nil
	}
	var warns []convert.Warning
	switch a.Type {
	case "basic":
		user, w1 := convert.ParseText(a.Username)
		pass, w2 := convert.ParseText(a.Password)
		e.BasicAuth = &syntax.BasicAuth{User: user, Password: pass}
		warns = append(append(warns, w1...), w2...)
	case "bearer":
		token, w := convert.ParseText(a.Token)
		warns = append(warns, w...)
		value := append(syntax.Text{syntax.Lit("Bearer ")}, token...)
		e.Headers = append(e.Headers, syntax.Field{Key: syntax.PlainText("Authorization"), Value: value})
	case "apikey":
		key, kw := convert.ParseText(a.Key)
		value, vw := convert.ParseText(a.Value)
		warns = append(append(warns, kw...), vw...)
		field := syntax.Field{Key: key, Value: value}
		if a.Placement == "query" {
			e.Query = append(e.Query, field)
		} else {
			e.Headers = append(e.Headers, field)
		}
	default:
		warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedAuth,
			Message: fmt.Sprintf("%s: auth type %q has no Sonde equivalent; the request is written without auth", name, a.Type)})
	}
	return warns
}
