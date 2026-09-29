// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build server && !e2eharness

// Command sonde-desktop-server serves the Sonde app to a browser on
// 127.0.0.1, for example on a remote machine reached through an SSH
// tunnel. Only a browser that opened the launch link can use it.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/browser"

	"github.com/nhtera/sonde/desktop/internal/serverauth"
	"github.com/nhtera/sonde/internal/sandbox"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sonde-desktop-server:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("sonde-desktop-server", flag.ContinueOnError)
	rootFlag := fs.String("root", ".", "project folder")
	portFlag := fs.Int("port", 0, "port on 127.0.0.1 (0: a free port); an SSH tunnel must forward the same local port")
	open := fs.Bool("open", false, "open the app in the default browser")
	data := fs.String("data", "", "app data folder (default: Sonde in the user config and cache folders)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	root, err := projectRoot(*rootFlag)
	if err != nil {
		return err
	}
	port, err := checkPort(*portFlag)
	if err != nil {
		return err
	}
	dirs, err := openDirs(*data)
	if err != nil {
		return err
	}
	defer dirs.Close()
	guard, err := serverauth.New(port)
	if err != nil {
		return err
	}
	link := &launchLink{dir: dirs.Cache(), guard: guard, port: port}
	defer link.remove()
	app := newServerApp(&Host{Mode: ModeServer, Root: root, Dirs: dirs}, port, guard)

	// The link is minted once the port is ours: a link to a port another
	// program took would hand that program the nonce.
	go func() {
		if err := waitListening(port, 30*time.Second); err != nil {
			fmt.Fprintln(stdout, err)
			return
		}
		path, err := link.mint()
		if err != nil {
			fmt.Fprintln(stdout, "launch link:", err)
			return
		}
		fmt.Fprintf(stdout, "Sonde is serving %s on http://%s:%d/\n", root, loopback, port)
		fmt.Fprintf(stdout, "Open %s in a browser, or copy the link inside it.\n", path)
		fmt.Fprintln(stdout, "The link works once, within 60 seconds; press Enter for a new one.")
		if *open {
			if err := browser.OpenFile(path); err != nil {
				fmt.Fprintln(stdout, "could not open a browser:", err)
			}
		}
		lines := bufio.NewScanner(stdin)
		for lines.Scan() {
			if path, err := link.mint(); err != nil {
				fmt.Fprintln(stdout, "new link:", err)
			} else {
				fmt.Fprintf(stdout, "New link in %s\n", path)
			}
		}
	}()
	return app.Run()
}

// launchLink keeps the launch link in a file only the user can read: a
// page that opens the one-time URL, so the nonce is never on a command
// line.
type launchLink struct {
	dir   *sandbox.Root
	guard *serverauth.Guard
	port  int

	mu  sync.Mutex // serializes mint
	gen atomic.Int64
}

func (l *launchLink) name() string { return "launch-" + strconv.Itoa(l.port) + ".html" }

// mint writes a link with a fresh nonce, revoking earlier links, and
// returns the file's path. The file is removed when that nonce is used or
// expires, unless a newer link replaced it.
func (l *launchLink) mint() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	gen := l.gen.Add(1)
	l.guard.Revoke()
	nonce, err := l.guard.Nonce(func() {
		if l.gen.Load() == gen {
			_ = l.dir.Remove(l.name())
		}
	})
	if err != nil {
		return "", err
	}
	url := html.EscapeString("http://" + loopback + ":" + strconv.Itoa(l.port) + "/?nonce=" + nonce)
	page := `<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=` + url +
		`"><title>Sonde</title><a href="` + url + `">` + url + "</a>\n"
	if err := l.dir.WriteFileAtomic(l.name(), []byte(page), 0o600); err != nil {
		return "", err
	}
	return filepath.Join(l.dir.Dir(), l.name()), nil
}

// remove revokes the link and deletes its file (on shutdown).
func (l *launchLink) remove() {
	l.gen.Add(1)
	l.guard.Revoke()
	_ = l.dir.Remove(l.name())
}
