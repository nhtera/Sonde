// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

func latest(t *testing.T, f *feed, channel, current string) (*found, error) {
	t.Helper()
	return f.latest(context.Background(), channel, mustVersion(t, current))
}

func TestFeedChannelsAndOrder(t *testing.T) {
	r := newRelServer(t)
	r.tag("v1.3.1", "editors/vscode/v1.0.3", "desktop/vbogus", "desktop/v0.3.0-rc.1")
	r.release("0.2.1", nil)
	r.release("0.1.0", nil)
	r.release("0.10.0-beta.1", nil)
	// Its GitHub prerelease flag is not read: the version says it.
	r.release("0.3.0-rc.1", nil)

	f, err := latest(t, r.hostFeed(), "stable", "0.1.0")
	if err != nil || f == nil || f.manifest.Version != "0.2.1" {
		t.Fatalf("stable: %+v, %v; want 0.2.1", f, err)
	}
	f, err = latest(t, r.hostFeed(), "prerelease", "0.2.1")
	if err != nil || f == nil || f.manifest.Version != "0.10.0-beta.1" {
		t.Fatalf("prerelease: %+v, %v; want 0.10.0-beta.1", f, err)
	}
	for _, cur := range []string{"0.2.1", "0.2.2"} {
		if f, err := latest(t, r.hostFeed(), "stable", cur); err != nil || f != nil {
			t.Errorf("stable from %s: %+v, %v; want up to date", cur, f, err)
		}
	}
}

func TestFeedPagination(t *testing.T) {
	r := newRelServer(t)
	r.pageSize = 60
	for i := range 150 {
		r.tag(fmt.Sprintf("v1.%d.0", i))
	}
	r.release("0.2.1", nil) // on the third page
	f, err := latest(t, r.hostFeed(), "stable", "0.2.0")
	if err != nil || f == nil || f.manifest.Version != "0.2.1" {
		t.Fatalf("%+v, %v; want 0.2.1 from page 3", f, err)
	}
}

func TestFeedErrors(t *testing.T) {
	cases := map[string]struct {
		setup func(r *relServer)
		kind  ErrorKind
	}{
		"no desktop tag": {func(r *relServer) { r.tag("v1.3.1") }, KindRelease},
		"manifest missing": {func(r *relServer) {
			r.release("0.2.1", nil)
			r.mu.Lock()
			delete(r.files, filePath("0.2.1", "Sonde-Desktop-0.2.1.update.json"))
			r.mu.Unlock()
		}, KindRelease},
		"manifest over 1 MiB": {func(r *relServer) {
			r.release("0.2.1", nil)
			r.put(filePath("0.2.1", "Sonde-Desktop-0.2.1.update.json"), []byte(strings.Repeat(" ", maxManifest+1)))
		}, KindRelease},
		"bad signature": {func(r *relServer) {
			r.release("0.2.1", func(m *manifest.Manifest) { m.Notes = "changed" })
		}, KindVerification},
		"unknown key": {func(r *relServer) {
			r.release("0.2.1", func(m *manifest.Manifest) { m.KeyID = "k9" })
		}, KindVerification},
		"version is not the tag": {func(r *relServer) {
			// 0.2.2's genuine manifest, published as 0.2.3's.
			m := r.release("0.2.2", nil)
			r.release("0.2.3", func(n *manifest.Manifest) { *n = m })
		}, KindVerification},
		"not JSON": {func(r *relServer) {
			r.release("0.2.1", nil)
			r.put(filePath("0.2.1", "Sonde-Desktop-0.2.1.update.json"), []byte("<html>"))
		}, KindVerification},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRelServer(t)
			c.setup(r)
			f, err := latest(t, r.hostFeed(), "stable", "0.1.0")
			if err == nil || f != nil {
				t.Fatalf("%+v, %v; want an error", f, err)
			}
			if k := kindOf(err); k != c.kind {
				t.Fatalf("kind %s (%v), want %s", k, err, c.kind)
			}
			if c.kind == KindVerification && !errors.Is(err, manifest.ErrVerification) {
				t.Fatalf("%v does not wrap ErrVerification", err)
			}
		})
	}
	t.Run("offline", func(t *testing.T) {
		r := newRelServer(t)
		fd := r.hostFeed()
		r.srv.Close()
		if _, err := latest(t, fd, "stable", "0.1.0"); kindOf(err) != KindNetwork {
			t.Fatalf("%v: kind %s, want network", err, kindOf(err))
		}
	})
}

func TestFeedPicksThisSystem(t *testing.T) {
	r := newRelServer(t)
	r.release("0.2.1", nil)
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "Sonde-Desktop-0.2.1-macos-universal.zip"},
		{"darwin", "amd64", "Sonde-Desktop-0.2.1-macos-universal.zip"},
		{"windows", "arm64", "Sonde-Desktop-0.2.1-windows-arm64-setup.exe"},
		{"linux", "amd64", "Sonde-Desktop-0.2.1-linux-x86_64.AppImage"},
	} {
		f, err := latest(t, r.feed(c.goos, c.goarch), "stable", "0.1.0")
		if err != nil || f.artifact.Filename != c.want {
			t.Errorf("%s/%s: %+v, %v", c.goos, c.goarch, f, err)
		}
	}
	if _, err := latest(t, r.feed("linux", "arm64"), "stable", "0.1.0"); kindOf(err) != KindRelease {
		t.Errorf("linux/arm64: %v, want a release error", err)
	}
}

func TestEndpoints(t *testing.T) {
	for _, ok := range []string{"https://updates.example", "http://127.0.0.1:8080", "http://localhost:1", "http://[::1]:9/"} {
		if _, err := testEndpoints(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://example.com", "ftp://127.0.0.1", "https://u:p@example.com", "127.0.0.1:8080", "https://h/?q=1"} {
		if _, err := testEndpoints(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	gh := githubEndpoints()
	for u, want := range map[string]bool{
		"https://api.github.com/x": true, "https://github.com/x": true,
		"https://release-assets.githubusercontent.com/x": true, "https://objects.githubusercontent.com/x": true,
		"http://github.com/x": false, "https://github.com.evil.example/x": false, "https://evilgithubusercontent.com/x": false,
	} {
		if got := gh.allowed(mustURL(t, u)); got != want {
			t.Errorf("allowed(%s) = %v", u, got)
		}
	}
}

func TestProxyURL(t *testing.T) {
	for raw, want := range map[string]string{ //nolint:gosec // G101: test proxy URLs
		"10.0.0.1:3128":            "http://10.0.0.1:3128",
		"proxy.corp:8080":          "http://proxy.corp:8080",
		"https://p.example:443":    "https://p.example:443",
		"socks5://127.0.0.1:1080":  "socks5://127.0.0.1:1080",
		"http://u:pw@p.example:80": "http://u:pw@p.example:80",
	} {
		u, err := proxyURL(raw)
		if err != nil || u.String() != want {
			t.Errorf("proxyURL(%q) = %v, %v; want %s", raw, u, err, want)
		}
	}
	_, err := proxyURL("http://u:secret-pw@%zz")
	if err == nil || strings.Contains(err.Error(), "secret-pw") {
		t.Fatalf("error %v: want one that never repeats the setting", err)
	}
}
