// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// fakeHost is the Wails application, for a real updater.
type fakeHost struct{}

func (fakeHost) Emit(string, ...any) bool                              { return true }
func (fakeHost) OnEvent(string, func(any)) func()                      { return func() {} }
func (fakeHost) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (fakeHost) Quit()                                                 {}

// stage runs the real Wails updater on the release the feed verified for a
// target: Check through Sonde's provider, then DownloadAndInstall.
func stage(t *testing.T, r *relServer, goos, goarch string) (string, error) {
	t.Helper()
	f, err := r.feed(goos, goarch).latest(context.Background(), "stable", mustVersion(t, "0.1.0"))
	if err != nil {
		return "", err
	}
	ep := r.endpoints()
	p := &provider{ep: ep, client: newClient(ep, func() string { return "" })}
	p.offer(f, nil)
	u := updater.New(fakeHost{})
	if err := u.Init(updater.Config{CurrentVersion: "0.1.0", Providers: []updater.Provider{p}, Window: updater.WindowNone, Platform: goos, Arch: goarch}); err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil || rel == nil || rel.Version != "9.9.9" {
		t.Fatalf("Check = %+v, %v", rel, err)
	}
	if err := u.DownloadAndInstall(context.Background()); err != nil {
		return "", err
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(u.DownloadedPath())) })
	return u.DownloadedPath(), nil
}

func TestEndToEnd(t *testing.T) {
	for _, tg := range [][2]string{{"darwin", "arm64"}, {"darwin", "amd64"}, {"windows", "amd64"}, {"linux", "amd64"}} {
		t.Run(tg[0]+"/"+tg[1], func(t *testing.T) {
			r := newRelServer(t)
			r.release("9.9.9", nil)
			staged, err := stage(t, r, tg[0], tg[1])
			if err != nil {
				t.Fatal(err)
			}
			if tg[0] == "darwin" {
				b, err := os.ReadFile(filepath.Join(staged, "Contents", "MacOS", "sonde-desktop"))
				if filepath.Base(staged) != "Sonde.app" || err != nil || string(b) != "sonde-desktop 9.9.9" {
					t.Fatalf("staged %s: %q, %v", staged, b, err)
				}
				return
			}
			b, err := os.ReadFile(staged)
			name := updateFile("9.9.9", tg[0], tg[1])
			if err != nil || filepath.Base(staged) != name || string(b) != name+" bytes" {
				t.Fatalf("staged %s: %q, %v", staged, b, err)
			}
		})
	}
}

func TestEndToEndRefusals(t *testing.T) {
	name := updateFile("9.9.9", "linux", "amd64")
	cases := map[string]func(r *relServer){
		"a flipped byte": func(r *relServer) {
			r.release("9.9.9", nil)
			b := []byte(name + " bytes")
			b[0] ^= 1
			r.put(filePath("9.9.9", name), b)
		},
		"more bytes than signed": func(r *relServer) {
			r.release("9.9.9", nil)
			r.put(filePath("9.9.9", name), []byte(name+" bytes and more"))
		},
		"fewer bytes than signed": func(r *relServer) {
			r.release("9.9.9", nil)
			r.put(filePath("9.9.9", name), []byte(name))
		},
		"a manifest signed by another key": func(r *relServer) {
			other := newRelServer(t)
			m := other.release("9.9.9", nil)
			r.release("9.9.9", func(n *manifest.Manifest) { *n = m })
		},
	}
	for what, setup := range cases {
		t.Run(what, func(t *testing.T) {
			r := newRelServer(t)
			setup(r)
			if staged, err := stage(t, r, "linux", "amd64"); err == nil {
				t.Fatalf("staged %s", staged)
			}
		})
	}
}

func TestProviderCapsAtTheSignedSize(t *testing.T) {
	r := newRelServer(t)
	r.release("9.9.9", nil)
	name := updateFile("9.9.9", "linux", "amd64")
	huge := bytes.Repeat([]byte("x"), 1<<20)
	r.put(filePath("9.9.9", name), huge)
	f, err := r.feed("linux", "amd64").latest(context.Background(), "stable", mustVersion(t, "0.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	ep := r.endpoints()
	p := &provider{ep: ep, client: newClient(ep, func() string { return "" })}
	p.offer(f, nil)
	rel, err := p.Check(context.Background(), updater.CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got countWriter
	if err := p.Download(context.Background(), rel, &got, func(int64, int64) {}); kindOf(err) != KindVerification {
		t.Fatalf("Download = %v, want a verification error", err)
	}
	if got.n > f.artifact.Size {
		t.Fatalf("wrote %d bytes past the signed %d", got.n, f.artifact.Size)
	}
	// A release other than the verified one is never downloaded.
	rel.Artifact.Filename = "other.exe"
	if err := p.Download(context.Background(), rel, io.Discard, func(int64, int64) {}); err == nil {
		t.Fatal("downloaded a release that was not verified")
	}
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }
