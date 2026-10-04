// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// relServer is a fake GitHub: the matching-refs tag list (paged) and
// release downloads, with manifests signed by a test key.
type relServer struct {
	t        *testing.T
	srv      *httptest.Server
	priv     ed25519.PrivateKey
	pub      ed25519.PublicKey
	pageSize int
	hits     atomic.Int64

	mu    sync.Mutex
	refs  []string          // "refs/tags/..."
	files map[string][]byte // URL path -> body
}

func newRelServer(t *testing.T) *relServer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &relServer{t: t, priv: priv, pub: pub, pageSize: 100, files: map[string][]byte{}}
	r.srv = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.srv.Close)
	return r
}

const refsPath = "/repos/nhtera/Sonde/git/matching-refs/tags/desktop/v"

func (r *relServer) serve(w http.ResponseWriter, req *http.Request) {
	r.hits.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.URL.Path == refsPath {
		page, _ := strconv.Atoi(req.URL.Query().Get("page"))
		page = max(page, 1)
		lo := min((page-1)*r.pageSize, len(r.refs))
		hi := min(lo+r.pageSize, len(r.refs))
		if hi < len(r.refs) {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?per_page=%d&page=%d>; rel="next", <%s%s?page=1>; rel="first"`, r.srv.URL, refsPath, r.pageSize, page+1, r.srv.URL, refsPath))
		}
		out := []map[string]string{}
		for _, ref := range r.refs[lo:hi] {
			out = append(out, map[string]string{"ref": ref, "url": "x"})
		}
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	body, ok := r.files[req.URL.Path]
	if !ok {
		http.NotFound(w, req)
		return
	}
	_, _ = w.Write(body)
}

// tag adds tag refs (desktop/v0.2.0, v1.3.1, ...).
func (r *relServer) tag(tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		r.refs = append(r.refs, "refs/tags/"+t)
	}
}

func filePath(v, name string) string {
	return "/nhtera/Sonde/releases/download/desktop/v" + v + "/" + name
}

// updateFile is the update file of version v for a target, by its
// release name.
func updateFile(v, goos, arch string) string {
	switch goos {
	case "darwin":
		return "Sonde-Desktop-" + v + "-macos-universal.zip"
	case "windows":
		return "Sonde-Desktop-" + v + "-windows-" + arch + "-setup.exe"
	}
	return "Sonde-Desktop-" + v + "-linux-x86_64.AppImage"
}

// release publishes desktop/v<v>: its four update files (content "<name>
// bytes") and a manifest signed by the test key, then changed by edit.
// It returns the manifest as published.
func (r *relServer) release(v string, edit func(m *manifest.Manifest)) manifest.Manifest {
	r.t.Helper()
	m := manifest.Manifest{Schema: manifest.Schema, Version: v, Notes: "Sonde Desktop " + v}
	for _, tg := range [][2]string{{"darwin", "universal"}, {"windows", "amd64"}, {"windows", "arm64"}, {"linux", "amd64"}} {
		name := updateFile(v, tg[0], tg[1])
		body := []byte(name + " bytes")
		if tg[0] == "darwin" {
			body = appZip(r.t, v)
		}
		sum := sha512.Sum512(body)
		m.Artifacts = append(m.Artifacts, manifest.Artifact{Platform: tg[0], Arch: tg[1], Filename: name, Size: int64(len(body)), SHA512: hex.EncodeToString(sum[:])})
		r.put(filePath(v, name), body)
	}
	m, err := manifest.Sign(m, r.priv, "k1")
	if err != nil {
		r.t.Fatal(err)
	}
	if edit != nil {
		edit(&m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		r.t.Fatal(err)
	}
	r.put(filePath(v, "Sonde-Desktop-"+v+".update.json"), b)
	r.tag("desktop/v" + v)
	return m
}

func (r *relServer) put(path string, body []byte) {
	r.mu.Lock()
	r.files[path] = body
	r.mu.Unlock()
}

func (r *relServer) endpoints() endpoints {
	ep, err := testEndpoints(r.srv.URL)
	if err != nil {
		r.t.Fatal(err)
	}
	return ep
}

func (r *relServer) feed(goos, goarch string) *feed {
	ep := r.endpoints()
	return &feed{ep: ep, client: newClient(ep, func() string { return "" }), keys: map[string]ed25519.PublicKey{"k1": r.pub}, goos: goos, goarch: goarch}
}

func (r *relServer) hostFeed() *feed { return r.feed(runtime.GOOS, runtime.GOARCH) }

func mustVersion(t *testing.T, s string) version {
	t.Helper()
	v, ok := parseVersion(s)
	if !ok {
		t.Fatalf("version %q", s)
	}
	return v
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// appZip is a macOS update archive: Sonde.app alone, its binary saying v.
func appZip(t *testing.T, v string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create("Sonde.app/Contents/MacOS/sonde-desktop")
	if err == nil {
		_, err = w.Write([]byte("sonde-desktop " + v))
	}
	if err == nil {
		err = z.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
