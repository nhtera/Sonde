// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestHelperModeFirst: an update's helper is the app's own binary started
// with the updater's environment; it must exit before any of Sonde's
// startup (flags, data folders, the instance lock) runs.
func TestHelperModeFirst(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" || fn.Recv != nil {
			continue
		}
		if len(fn.Body.List) > 0 {
			if es, ok := fn.Body.List[0].(*ast.ExprStmt); ok {
				if call, ok := es.X.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HandleHelperMode" {
						if id, ok := sel.X.(*ast.Ident); ok && id.Name == "updater" {
							return
						}
					}
				}
			}
		}
		t.Fatal("main's first statement is not updater.HandleHelperMode()")
	}
	t.Fatal("main.go has no main function")
}
