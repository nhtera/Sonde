// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package wsession

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/goleak"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/fixture"
	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
)

const secret = "ws-sentinel-73" //nolint:gosec // G101: test sentinel

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// codeOf is err's apperr code ("" for none).
func codeOf(err error) string {
	if e, ok := errors.AsType[*apperr.Error](err); ok {
		return e.Code
	}
	return ""
}

// sessions returns a service dialing the fixture's WebSocket echo, with a
// secret the page sends back.
func sessions(t *testing.T) (*Sessions, *emit.Recorder) {
	t.Helper()
	srv := httptest.NewServer(fixture.New())
	t.Cleanup(srv.Close)
	ws := "ws" + strings.TrimPrefix(srv.URL, "http")
	rec := &emit.Recorder{}
	prepare := func(_ context.Context, file, source, _ string) (*engine.Runner, engine.Job, error) {
		r := engine.NewRunner(engine.Options{Variables: map[string]any{"ws": ws}, Secrets: map[string]string{"tok": secret}})
		enginex.EnableHostEvents(r)
		return r, engine.Job{Name: filepath.Join(t.TempDir(), file), Source: []byte(source)}, nil
	}
	return New(rec, prepare), rec
}

const src = "GET {{ws}}/ws\nX-Token: {{tok}}\n[SondeMessages]\nsend: `scripted`\nreceive\nHTTP 101\n"

// waitFor waits until the recorder holds n events of topic.
func waitFor(t *testing.T, rec *emit.Recorder, topic string, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got []Event
		for _, e := range rec.Events() {
			if e.Topic == topic {
				got = append(got, e.Data.(Event))
			}
		}
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSession sends a text and a binary message on the echo, gets both
// back redacted (the scripted steps do not run), then closes.
func TestSession(t *testing.T) {
	m, rec := sessions(t)
	opened, err := m.Open(context.Background(), OpenRequest{SessionID: "s1", File: "live.sonde", Source: src, Entry: 1})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Status != 101 || !strings.HasSuffix(opened.URL, "/ws") {
		t.Errorf("opened %+v", opened)
	}
	if err := m.Send("s1", "hello "+secret, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, rec, TopicPrefix+"s1", 2) // its echo first
	if err := m.Send("s1", "01 ff", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Send("s1", "zz", true); codeOf(err) != apperr.Invalid {
		t.Errorf("bad hex: %v", err)
	}
	waitFor(t, rec, TopicPrefix+"s1", 4)
	m.Close("s1")
	evs := waitFor(t, rec, TopicPrefix+"s1", 5)
	if len(evs) != 5 {
		t.Fatalf("%d events: %+v", len(evs), evs)
	}
	for _, e := range evs[:4] {
		if e.Type != TypeMessage || e.Message == nil {
			t.Fatalf("event %+v", e)
		}
		redactcheck.AssertNoSecret(t, "message", e, secret)
	}
	if m0 := evs[0].Message; m0.Direction != "sent" || m0.Data != "hello ***" {
		t.Errorf("first message %+v", m0)
	}
	if m1 := evs[1].Message; m1.Direction != "received" || m1.Data != "hello ***" {
		t.Errorf("echo %+v", m1)
	}
	if m3 := evs[3].Message; !m3.Binary || m3.Data != "Af8=" {
		t.Errorf("binary echo %+v", m3)
	}
	if last := evs[4]; last.Type != TypeClosed || last.Error != "" {
		t.Errorf("closed %+v", last)
	}
	if err := m.Send("s1", "late", false); codeOf(err) != apperr.NotFound {
		t.Errorf("send after close: %v", err)
	}
}

// TestOpenRefused: a non-WebSocket entry, a taken id and a bad id are
// refused; nothing stays open.
func TestOpenRefused(t *testing.T) {
	m, _ := sessions(t)
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "a", File: "t.sonde", Source: "GET {{ws}}/health\n", Entry: 1}); err == nil {
		t.Error("a plain request opened a session")
	}
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "a:b", File: "t.sonde", Source: src, Entry: 1}); codeOf(err) != apperr.Invalid {
		t.Errorf("bad id: %v", err)
	}
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "b", File: "t.sonde", Source: src, Entry: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "b", File: "t.sonde", Source: src, Entry: 1}); codeOf(err) != apperr.Busy {
		t.Errorf("taken id: %v", err)
	}
	m.CloseAll()
	if len(m.open) != 0 {
		t.Errorf("%d sessions still open", len(m.open))
	}
}

// TestServerCloses: a server that closes the connection ends the
// session, with its close code as the reason.
func TestServerCloses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, _, _ = c.Read(r.Context())
		_ = c.Close(websocket.StatusCode(4000), "bye")
	}))
	t.Cleanup(srv.Close)
	rec := &emit.Recorder{}
	m := New(rec, func(_ context.Context, file, source, _ string) (*engine.Runner, engine.Job, error) {
		r := engine.NewRunner(engine.Options{Variables: map[string]any{"ws": "ws" + strings.TrimPrefix(srv.URL, "http")}, Secrets: map[string]string{"tok": secret}})
		enginex.EnableHostEvents(r)
		return r, engine.Job{Name: filepath.Join(t.TempDir(), file), Source: []byte(source)}, nil
	})
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "c", File: "c.sonde", Source: src, Entry: 1}); err != nil {
		t.Fatal(err)
	}
	if err := m.Send("c", "hi", false); err != nil {
		t.Fatal(err)
	}
	evs := waitFor(t, rec, TopicPrefix+"c", 2)
	if last := evs[len(evs)-1]; last.Type != TypeClosed || !strings.Contains(last.Error, "4000") {
		t.Fatalf("events %+v", evs)
	}
	if err := m.Send("c", "late", false); codeOf(err) != apperr.NotFound {
		t.Errorf("send after the server closed: %v", err)
	}
}

// TestCloseWhileOpening: a Close before the dial ends leaves nothing open
// (the page closed the panel while the session was opening).
func TestCloseWhileOpening(t *testing.T) {
	m, _ := sessions(t)
	inner := m.prepare
	planned := make(chan struct{})
	release := make(chan struct{})
	m.prepare = func(ctx context.Context, file, source, env string) (*engine.Runner, engine.Job, error) {
		close(planned)
		<-release
		return inner(ctx, file, source, env)
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.Open(context.Background(), OpenRequest{SessionID: "p", File: "p.sonde", Source: src, Entry: 1})
		done <- err
	}()
	<-planned
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "p", File: "p.sonde", Source: src, Entry: 1}); codeOf(err) != apperr.Busy {
		t.Errorf("the id is not reserved while dialing: %v", err)
	}
	m.Close("p")
	close(release)
	if err := <-done; err == nil {
		t.Error("an Open closed while dialing succeeded")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.open) != 0 {
		t.Errorf("%d sessions left open", len(m.open))
	}
}
