// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// streamResult is a WebSocket entry whose messages hold the run's secret.
func streamResult(n int) *engine.UnitResult {
	res := successResult("ws.sonde")
	s := &exchange.Stream{Protocol: exchange.ProtocolWebSocket, StopReason: exchange.StopScript}
	s.Messages = append(s.Messages,
		exchange.Message{Direction: exchange.Sent, Data: []byte(`{"token": "` + testSecret + `"}`), At: time.Millisecond},
		exchange.Message{Binary: true, Data: []byte{0, 1}, At: 2 * time.Millisecond})
	for i := range n {
		s.Messages = append(s.Messages, exchange.Message{Data: []byte(strings.Repeat("x", i%3*2000)), At: 3 * time.Millisecond})
	}
	res.Entries[0].Calls[0].Response.Stream = s
	return res
}

func TestJSONStream(t *testing.T) {
	jr, err := JSON(streamResult(0), redactTestSecret, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(jr.Entries[0].Sonde)
	want := `{"stream":{"messages":[{"data":"{\"token\": \"***\"}","direction":"sent","time":1},` +
		`{"binary":true,"data":"AAE=","direction":"received","time":2}],"protocol":"websocket","received":1,"sent":1,"stop_reason":"script"}}`
	if string(b) != want {
		t.Errorf("sonde =\n%s\nwant\n%s", b, want)
	}
}

// TestJSONStreamBinaryRedacted checks a secret in a binary frame is
// masked before the frame is base64-encoded.
func TestJSONStreamBinaryRedacted(t *testing.T) {
	res := streamResult(0)
	s := res.Entries[0].Calls[0].Response.Stream
	s.Messages = append(s.Messages, exchange.Message{Binary: true, Data: []byte("k=" + testSecret)})
	jr, err := JSON(res, redactTestSecret, nil)
	if err != nil {
		t.Fatal(err)
	}
	msgs := jr.Entries[0].Sonde.Stream.Messages
	if got := msgs[len(msgs)-1].Data; got != base64.StdEncoding.EncodeToString([]byte("k=***")) {
		t.Errorf("binary data = %q", got)
	}
}

func TestHTMLStream(t *testing.T) {
	dir := t.TempDir()
	if err := WriteHTML(dir, []*engine.UnitResult{streamResult(120)}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	pages, _ := filepath.Glob(filepath.Join(dir, "store", "*.html"))
	if len(pages) != 1 {
		t.Fatalf("pages = %v", pages)
	}
	b, err := os.ReadFile(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{"Stream: websocket, 1 sent, 121 received, stopped by script", "base64: AAE=",
		"&hellip; 22 more messages", "... (4000 bytes)", "***"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, testSecret) {
		t.Error("page leaks the secret")
	}
}
