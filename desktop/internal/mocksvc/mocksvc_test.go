// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mocksvc

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/sandbox"
)

const spec = `openapi: 3.0.3
info: {title: pets, version: "1"}
paths:
  /pets:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: array, items: {type: string}}
              example: ["rex"]
  /pets/{id}:
    get:
      parameters: [{name: id, in: path, required: true, schema: {type: string}}]
      responses:
        "200": {description: ok, content: {application/json: {example: {name: rex}}}}
`

type recorder struct {
	mu     sync.Mutex
	topics []string
	data   []any
}

func (r *recorder) emit(topic string, data any) {
	r.mu.Lock()
	r.topics, r.data = append(r.topics, topic), append(r.data, data)
	r.mu.Unlock()
}

func (r *recorder) logs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for i, t := range r.topics {
		if t == TopicLog {
			out = append(out, r.data[i].(string))
		}
	}
	return out
}

func setup(t *testing.T) (*Mocks, *recorder, *string) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"openapi.yaml": spec, "sonde.yaml": "version: 1\nopenapi:\n  spec: openapi.yaml\nenvironments: {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, _ := sandbox.Open(dir)
	rec := &recorder{}
	var base string
	m := New(rec.emit, func() *sandbox.Root { return root }, func(u string) { base = u })
	t.Cleanup(m.Stop)
	return m, rec, &base
}

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestSpecStartStop(t *testing.T) {
	m, rec, base := setup(t)
	s, err := m.Spec(context.Background())
	if err != nil || s.File != "openapi.yaml" || len(s.Operations) != 2 {
		t.Fatalf("spec %+v %v", s, err)
	}
	port := freePort(t)
	st, err := m.Start(context.Background(), port)
	if err != nil {
		t.Fatal(err)
	}
	if st.URL != "http://127.0.0.1:"+strconv.Itoa(port) || *base != st.URL {
		t.Errorf("status %+v, base_url %q", st, *base)
	}
	res, err := http.Get(st.URL + "/pets")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "rex") {
		t.Errorf("mock answered %d %s", res.StatusCode, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(rec.logs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if logs := rec.logs(); len(logs) == 0 || !strings.Contains(logs[0], "/pets") {
		t.Errorf("log %v", logs)
	}
	var ae *apperr.Error
	if _, err := m.Start(context.Background(), port); !errors.As(err, &ae) || ae.Code != apperr.Busy {
		t.Errorf("second start: %v", err)
	}
	m.Stop()
	if *base != "" || m.Status().Running {
		t.Errorf("after stop: base %q, %+v", *base, m.Status())
	}
	if _, err := http.Get(st.URL + "/pets"); err == nil {
		t.Error("the mock still answers after Stop")
	}
}

func TestLoopbackOnlyAndPortInUse(t *testing.T) {
	m, _, _ := setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	_, err = m.Start(context.Background(), port)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.Busy || ae.Data.(map[string]int)["next"] <= port {
		t.Fatalf("port in use: %v %+v", err, ae)
	}
	st, err := m.Start(context.Background(), ae.Data.(map[string]int)["next"])
	if err != nil {
		t.Fatal(err)
	}
	host, _, _ := net.SplitHostPort(strings.TrimPrefix(st.URL, "http://"))
	if host != "127.0.0.1" {
		t.Errorf("the mock listens on %s", host)
	}
}

func TestCoverage(t *testing.T) {
	m, _, _ := setup(t)
	if _, err := m.Spec(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Observe([]*engine.UnitResult{{Entries: []*engine.EntryResult{{Calls: []engine.Call{
		{Request: exchange.Request{Method: "GET", URL: "http://api.example/pets/7"}},
		{Request: exchange.Request{Method: "POST", URL: "http://api.example/pets"}},
	}}}}})
	c, err := m.Coverage(context.Background())
	if err != nil || c.Total != 2 || len(c.Covered) != 1 || c.Covered[0] != "GET /pets/{id}" {
		t.Errorf("coverage %+v %v", c, err)
	}
}
