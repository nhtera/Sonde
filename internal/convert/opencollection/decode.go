// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// maxItemDepth bounds "items" nesting during decodeItemsFrom below.
//
// A single top-level yaml.Decode call is naturally protected against a
// self-referential anchor ("&a {items: [*a]}") by go-yaml's own built-in
// alias-ratio guard (decode.go's aliasCount/allowedAliasRatio), because
// the whole tree is decoded within one decoder's bookkeeping. Decoding
// each item's node independently — required for M9's per-item isolation,
// so one malformed sibling item doesn't take the rest of the document
// down with it — cannot reuse that guard: every yaml.Node.Decode call
// starts a *fresh* decoder (see go.yaml.in/yaml/v3's Node.Decode), which
// resets the ratio bookkeeping to zero each time. Without maxItemDepth, a
// crafted cyclic alias inside "items" would recurse forever through
// decodeItemNode -> decodeItemsFrom -> decodeItemNode -> ... until the
// goroutine stack overflows (found by this package's own fuzz corpus).
const maxItemDepth = 200

// itemFields is item's shape minus Items: a single Decode into it can
// never recurse back into decodeItemsFrom, so — like any other struct in
// this package — it is safe to decode directly, bounded by go-yaml's own
// alias-ratio guard for whatever aliasing exists inside these fields.
type itemFields struct {
	Info     itemInfo      `yaml:"info"`
	Request  requestDefs   `yaml:"request"`
	Docs     description   `yaml:"docs"`
	HTTP     *httpBlock    `yaml:"http"`
	GraphQL  *graphqlBlock `yaml:"graphql"`
	Runtime  runtimeBlock  `yaml:"runtime"`
	Settings settings      `yaml:"settings"`
	Items    yaml.Node     `yaml:"items"` // walked by decodeItemsFrom, never Decode()d as items itself
}

// decodeDocumentNode builds a document from root, a bare yaml.Node
// already parsed (never itself decoded through reflection into item-
// shaped types — see decodeItemNode). Used for both the single-file
// layout's root document and, identically, a directory layout's
// opencollection.yml root file, whose "items" (if any) nest the same way.
func decodeDocumentNode(root *yaml.Node) (*document, error) {
	var f struct {
		OpenCollection string      `yaml:"opencollection"`
		Info           rootInfo    `yaml:"info"`
		Config         collConfig  `yaml:"config"`
		Request        requestDefs `yaml:"request"`
		Docs           description `yaml:"docs"`
		Items          yaml.Node   `yaml:"items"`
	}
	if err := root.Decode(&f); err != nil {
		return nil, err
	}
	doc := &document{OpenCollection: f.OpenCollection, Info: f.Info, Config: f.Config, Request: f.Request, Docs: f.Docs}
	doc.Items, doc.decodeErrors = decodeItemsFrom(&f.Items, 1)
	return doc, nil
}

// decodeItemNode builds one item from node (a mapping, or an alias
// resolving to one). Its own nested "items", if any, are decoded
// separately by decodeItemsFrom at depth+1 rather than as part of this
// Decode call — see maxItemDepth. A node that fails to decode at all
// becomes a zero item with a reason recorded, not an error: its siblings
// still get their chance (mapping doc, "Pinned version").
func decodeItemNode(node *yaml.Node, depth int) (item, []string) {
	var f itemFields
	if err := node.Decode(&f); err != nil {
		return item{}, []string{fmt.Sprintf("%v; item skipped", err)}
	}
	it := item{Info: f.Info, Request: f.Request, Docs: f.Docs,
		HTTP: f.HTTP, GraphQL: f.GraphQL, Runtime: f.Runtime, Settings: f.Settings}
	if depth > maxItemDepth {
		return it, []string{fmt.Sprintf("items nesting exceeds %d levels; deeper items dropped", maxItemDepth)}
	}
	it.Items, it.decodeErrors = decodeItemsFrom(&f.Items, depth+1)
	return it, nil
}

// decodeItemsFrom reads node's sequence elements directly (node.Content),
// never calling Decode on node itself or resolving it beyond one alias
// hop (resolveAlias): that is what keeps decodeItemNode's per-node Decode
// calls from ever being handed the same cyclic node twice at the same
// depth. An absent or non-sequence "items" is simply no items.
func decodeItemsFrom(node *yaml.Node, depth int) ([]item, []string) {
	resolved := resolveAlias(node)
	if resolved == nil || resolved.Kind != yaml.SequenceNode {
		return nil, nil
	}
	out := make([]item, 0, len(resolved.Content))
	var reasons []string
	for i, n := range resolved.Content {
		it, r := decodeItemNode(n, depth)
		for _, rr := range r {
			reasons = append(reasons, fmt.Sprintf("items[%d]: %s", i, rr))
		}
		out = append(out, it)
	}
	return out, reasons
}

// resolveAlias follows at most a handful of alias hops (a real document
// never chains more than one) and gives up rather than loop if it somehow
// doesn't reach a non-alias node.
func resolveAlias(n *yaml.Node) *yaml.Node {
	for hops := 0; n != nil && n.Kind == yaml.AliasNode; hops++ {
		if hops > 10 {
			return nil
		}
		n = n.Alias
	}
	if n == nil || n.Kind == 0 {
		return nil
	}
	return n
}

// UnmarshalYAML decodes v leniently: an entry that isn't even a mapping
// (or whose "name" isn't a plain string) decodes to a nameless zero value
// rather than failing, since every caller already drops a nameless or
// disabled variable; "secret" and "disabled" tolerate a quoted
// "true"/"false" a producer might write instead of a bare bool.
func (v *variable) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Name     string    `yaml:"name"`
		Secret   yaml.Node `yaml:"secret"`
		Disabled yaml.Node `yaml:"disabled"`
		Value    yaml.Node `yaml:"value"`
	}
	if value.Decode(&raw) != nil {
		return nil
	}
	v.Name = raw.Name
	v.Secret = tolerantBool(raw.Secret)
	v.Disabled = tolerantBool(raw.Disabled)
	v.Value = raw.Value
	return nil
}

// tolerantBool reads n as a bool, or as a "true"/"false" string
// (case-insensitive) some producers write instead; anything else is false.
func tolerantBool(n yaml.Node) bool {
	var b bool
	if n.Decode(&b) == nil {
		return b
	}
	var s string
	if n.Decode(&s) == nil {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return false
}
