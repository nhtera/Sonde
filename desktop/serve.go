// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build server

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/host"
	"github.com/nhtera/sonde/desktop/internal/serverauth"
)

// loopback is the only address server mode listens on.
const loopback = "127.0.0.1"

// newServerApp returns the app served over HTTP on loopback:port, every
// request passing guard. Wails' own event socket (/wails/events) is
// outside the guard, so app events go through the guarded event stream
// (emit.StreamPath) instead: h.Emit is set to it.
func newServerApp(h *host.Host, port int, guard *serverauth.Guard) (*application.App, error) {
	// Wails lets these override the address; server mode's address is fixed.
	_ = os.Unsetenv("WAILS_SERVER_HOST")
	_ = os.Unsetenv("WAILS_SERVER_PORT")
	stream := emit.NewStream()
	h.Emit = stream
	if err := h.Setup(); err != nil {
		return nil, err
	}
	opts := appOptions(h)
	opts.Services = append(opts.Services, application.NewServiceWithOptions(&eventStream{stream},
		application.ServiceOptions{Name: "events", Route: emit.StreamPath}))
	opts.Server = application.ServerOptions{
		Host:        loopback,
		Port:        port,
		ReadTimeout: time.Minute,
		// Binding calls answer when the work is done and body URLs stream
		// large responses, possibly through a slow tunnel: the Wails
		// default (30 s) would cut them off. Each request still ends with
		// its client.
		WriteTimeout: 12 * time.Hour,
	}
	opts.Assets.Middleware = guard.Middleware
	// Request logs would record launch links.
	opts.Assets.DisableLogging = true
	return application.New(opts), nil
}

// eventStream serves the event stream. It binds nothing: its only method
// is ServeHTTP, which Wails does not bind.
type eventStream struct{ s *emit.Stream }

func (e *eventStream) ServeHTTP(w http.ResponseWriter, r *http.Request) { e.s.ServeHTTP(w, r) }

// waitListening polls the server's health route until it answers or
// timeout passes.
func waitListening(port int, timeout time.Duration) error {
	url := "http://" + loopback + ":" + strconv.Itoa(port) + "/health"
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for {
		res, err := client.Get(url)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server did not start listening on port %d", port)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// freePort asks the OS for a free loopback port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", loopback+":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// checkPort validates a --port value; 0 picks a free port.
func checkPort(port int) (int, error) {
	if port == 0 {
		return freePort()
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("--port: %d is out of range", port)
	}
	return port, nil
}
