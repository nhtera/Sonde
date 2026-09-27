// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// shutdownGrace is how long in-flight requests may finish once the
// server is asked to stop.
const shutdownGrace = 5 * time.Second

// Serve serves h on ln until stop is closed, then shuts down gracefully:
// no new connections, in-flight requests get shutdownGrace to finish. It
// returns nil after a requested shutdown.
func Serve(stop <-chan struct{}, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-stop:
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	err := srv.Shutdown(ctx)
	if serr := <-errc; serr != nil && !errors.Is(serr, http.ErrServerClosed) {
		return serr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// Requests still running after the grace are cut off.
		return srv.Close()
	}
	return err
}
