// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"net"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/mock"
	"github.com/nhtera/sonde/internal/openapi"
)

type mockOptions struct {
	host             string
	port             int
	server           string
	validateRequests bool
	cors             bool
	allowRemote      bool
}

func newMockCmd() *cobra.Command {
	o := &mockOptions{}
	cmd := &cobra.Command{
		Use:   "mock [options] SPEC",
		Short: "Serve a mock of an OpenAPI spec",
		Long: "Mock serves the operations of an OpenAPI 3.x (or Swagger 2.0) spec over HTTP.\n" +
			"Each response is the lowest documented 2xx, or the status a\n" +
			"`Prefer: code=NNN` header asks for; its body is the example named by\n" +
			"`Prefer: example=NAME`, else the first documented example, else a\n" +
			"minimal instance generated from the schema. The same request always\n" +
			"gets the same bytes. Errors are application/problem+json: 404 for an\n" +
			"unknown path, 405 for an unknown method, 406 when no documented media\n" +
			"type matches Accept. It binds 127.0.0.1 unless --host says otherwise,\n" +
			"logs one line per request to stderr and stops on Ctrl-C or SIGTERM.\n" +
			"See docs/guides/mock-server.md.",
		Args:                  cobra.ExactArgs(1),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMock(cmd, o, args[0])
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.host, "host", "127.0.0.1", "address to bind (0.0.0.0 listens on every interface)")
	f.IntVar(&o.port, "port", 4010, "port to listen on (0 picks a free one)")
	f.StringVar(&o.server, "server", "", "serve under this server URL's base path instead of the spec's servers")
	f.BoolVar(&o.validateRequests, "validate-requests", false, "answer requests that do not match the spec with 415/422 problems")
	f.BoolVar(&o.cors, "cors", false, "answer CORS preflights and allow any origin")
	f.BoolVar(&o.allowRemote, "openapi-allow-remote", false, "allows a remote spec and remote $ref targets")
	return cmd
}

// runMock serves the spec until Ctrl-C (exit 130, like every command) or
// SIGTERM (exit 0). A spec that can not be loaded is a usage error, an
// address that can not be bound a runtime error.
func runMock(cmd *cobra.Command, o *mockOptions, specPath string) error {
	// SIGTERM is caught from the start: one arriving while a large spec
	// loads still exits 0.
	term, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	if o.host == "" {
		return invalidFlagErr("host", "HOST", o.host, fmt.Errorf("empty (use 0.0.0.0 to listen on every interface)"))
	}
	if o.port < 0 || o.port > 65535 {
		return invalidFlagErr("port", "PORT", strconv.Itoa(o.port), fmt.Errorf("not a port number"))
	}
	ctx := cmd.Context()
	loadCtx, cancelLoad := context.WithCancel(ctx)
	defer cancelLoad()
	defer context.AfterFunc(term, cancelLoad)()
	spec, err := openapi.Load(loadCtx, specPath, openapi.LoadOptions{AllowRemote: o.allowRemote})
	if term.Err() != nil {
		return nil
	}
	if err != nil {
		return NewExitError(ExitUsage, err)
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(o.host, strconv.Itoa(o.port)))
	if err != nil {
		return NewExitError(ExitRuntime, err)
	}
	stderr := cmd.ErrOrStderr()
	h := mock.Handler(spec.Mock(o.server), mock.Options{ValidateRequests: o.validateRequests, CORS: o.cors, Log: stderr})
	_, _ = fmt.Fprintf(stderr, "sonde mock: serving %d operation(s) of %s at http://%s (Ctrl-C to stop)\n",
		len(spec.Operations()), specPath, ln.Addr())

	stop := make(chan struct{})
	go func() {
		select {
		case <-stopFromContext(ctx):
		case <-term.Done():
		}
		close(stop)
	}()
	if err := mock.Serve(stop, ln, h); err != nil {
		return NewExitError(ExitRuntime, err)
	}
	return nil
}
