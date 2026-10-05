// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/closeguard"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/desktop/internal/settings"
)

// TestStablePathRealTags runs the stable channel against GitHub's real
// desktop tag list (testdata/desktop-tags.json, from
// `gh api repos/nhtera/Sonde/git/matching-refs/tags/desktop/v --paginate`),
// among CLI tags on several pages, with signed 0.1.9 and 0.2.0-rc.9
// releases added.
func TestStablePathRealTags(t *testing.T) {
	b, err := os.ReadFile("testdata/desktop-tags.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(b, &captured); err != nil || len(captured) == 0 {
		t.Fatalf("the captured tags: %v", err)
	}
	r := newRelServer(t)
	r.pageSize = 2
	r.tag("v1.3.0", "v1.3.1", "editors/vscode/v1.0.3")
	for _, c := range captured {
		r.tag(strings.TrimPrefix(c.Ref, "refs/tags/"))
	}
	r.release("0.1.9", nil)
	r.release("0.2.0-rc.9", nil)

	// An installed 0.1.9 on the stable channel: up to date, never the rc.
	st := settings.Open(sandboxtest.Open(t, t.TempDir()), &emit.Recorder{}, handles.New())
	m := New("0.1.9", st, &emit.Recorder{}, &closeguard.Guard{})
	m.now = func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }
	m.current, m.feed, m.prov = mustVersion(t, "0.1.9"), r.hostFeed(), &provider{}
	m.st = m.base(StateIdle)
	m.Check(true)
	if s := m.Status(); s.State != StateUpToDate {
		t.Fatalf("0.1.9 on stable: %+v; want up to date (the rc is not offered)", s)
	}

	// 0.2.0 is released: offered.
	r.release("0.2.0", nil)
	m.Check(true)
	if s := m.Status(); s.State != StateAvailable || s.Version != "0.2.0" {
		t.Fatalf("after 0.2.0: %+v; want 0.2.0 offered", s)
	}

	// No desktop tag at all: an error, not "up to date".
	empty := newRelServer(t)
	empty.tag("v1.3.1", "editors/vscode/v1.0.3")
	m.feed = empty.hostFeed()
	m.Check(true)
	if s := m.Status(); s.State != StateError || s.ErrorKind != KindRelease {
		t.Fatalf("no desktop tag: %+v; want a release error", s)
	}
}
