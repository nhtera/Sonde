// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package perftrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTraceWritesMeasuresAndQuits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.json")
	start := time.UnixMilli(1_000_000)
	quit := make(chan struct{})
	tr := New(path, `{"tour":true}`, start, func() { close(quit) })
	if tr.Plan() != `{"tour":true}` {
		t.Errorf("plan %q", tr.Plan())
	}
	tr.Interactive(1_001_250)
	tr.Record(Measure{Name: "tree scroll", Value: 60, Unit: "fps"})
	if err := tr.Done(); err != nil {
		t.Fatal(err)
	}
	<-quit
	var got struct{ Measures []Measure }
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil || len(got.Measures) != 2 || got.Measures[0].Value != 1250 {
		t.Errorf("%s %v", data, err)
	}
}

func TestOffDoesNothing(t *testing.T) {
	tr := New("", `{"tour":true}`, time.Now(), func() { t.Error("quit") })
	tr.Record(Measure{Name: "x"})
	if tr.Plan() != "" || tr.Done() != nil {
		t.Error("an untraced app runs a tour or writes")
	}
}
