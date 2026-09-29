// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"reflect"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nhtera/sonde/internal/runflags"
	"github.com/nhtera/sonde/internal/runplan"
)

// fill sets every field of v (a struct) to a non-zero value.
func fill(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	for i := range v.NumField() {
		f, name := v.Field(i), path+v.Type().Field(i).Name
		switch f.Kind() {
		case reflect.String:
			f.SetString("v-" + name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(7)
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"a-" + name, "b-" + name}))
		case reflect.Struct:
			fill(t, f, name+".")
		case reflect.Map:
		default:
			t.Fatalf("field %s: kind %s not covered", name, f.Kind())
		}
	}
}

// parse runs args (from runflags.Args) through the CLI's own flags.
func parse(t *testing.T, args []string) *runplan.Invocation {
	t.Helper()
	o := &runOptions{}
	cmd := &cobra.Command{Use: args[0]}
	addRunFlags(cmd, o)
	if err := cmd.ParseFlags(args[1:]); err != nil {
		t.Fatal(err)
	}
	return o.invocation(cmd, cmd.Flags().Args(), args[0] == "test")
}

// TestRunflagsRoundTrip renders an invocation with every field set and
// parses it back with the CLI's flags: the same invocation comes back.
func TestRunflagsRoundTrip(t *testing.T) {
	inv := &runplan.Invocation{}
	fill(t, reflect.ValueOf(inv).Elem(), "")
	inv.Cmd = "test"
	inv.Set = map[string]bool{}
	for _, name := range runflags.Names() {
		inv.Set[name] = true
	}
	got := parse(t, runflags.Args(inv))
	if !reflect.DeepEqual(got, inv) {
		t.Errorf("round trip:\n%+v\nwant\n%+v", got, inv)
	}

	// Every flag of `sonde run` but --version is rendered.
	cmd := &cobra.Command{}
	addRunFlags(cmd, &runOptions{})
	var cli []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "version" {
			cli = append(cli, f.Name)
		}
	})
	names := runflags.Names()
	slices.Sort(cli)
	slices.Sort(names)
	if !slices.Equal(cli, names) {
		t.Errorf("flags of sonde run:\n%v\nrendered:\n%v", cli, names)
	}
}

// TestRunflagsSamePlan checks that a partly given invocation renders to a
// command that builds the same run.
func TestRunflagsSamePlan(t *testing.T) {
	inv := &runplan.Invocation{
		Cmd: "run", Files: []string{"a.hurl"},
		Variables: []string{"host=x"}, Secrets: []string{"key=k"}, Header: []string{"X-A: 1"},
		MaxTime: "5", Retry: "3", Insecure: true, Proxy: "http://p:1", FromEntry: 2,
		Verbosity: "brief", OpenAPI: runplan.OpenAPI{Strict: true},
		Set: map[string]bool{"retry": true, "verbosity": true},
	}
	back := parse(t, runflags.Args(inv))
	want, err := runplan.New(inv, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := runplan.New(back, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Options, want.Options) || got.Workers != want.Workers || got.Repeat != want.Repeat {
		t.Errorf("plan:\n%+v\nwant\n%+v", got.Options, want.Options)
	}
}
