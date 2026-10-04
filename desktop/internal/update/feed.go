// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// ErrorKind says what went wrong, so the page can tell a refusal from
// being offline.
type ErrorKind string

// Error kinds.
const (
	// KindNetwork: offline, a timeout, GitHub's rate limit or a server error.
	KindNetwork ErrorKind = "network"
	// KindRelease: no desktop release at all, or one without a manifest or
	// an update file for this system.
	KindRelease ErrorKind = "release"
	// KindVerification: the manifest's signature, or a file's size or
	// digest, did not verify. Never silent.
	KindVerification ErrorKind = "verification"
	// KindInstall: the update could not be installed.
	KindInstall ErrorKind = "install"
)

// Error is an update failure of a kind.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func fail(kind ErrorKind, format string, a ...any) error {
	return &Error{Kind: kind, Err: fmt.Errorf(format, a...)}
}

// kindOf is err's kind; an unclassified error is a network one.
func kindOf(err error) ErrorKind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	if errors.Is(err, manifest.ErrVerification) {
		return KindVerification
	}
	return KindNetwork
}

// Limits on what the feed reads.
const (
	maxManifest = 1 << 20 // a manifest
	maxRefsPage = 4 << 20 // a page of tags
	maxRefPages = 50
)

// found is a verified release newer than the running app.
type found struct {
	manifest manifest.Manifest
	artifact manifest.Artifact // the update file for this system
}

// feed finds the newest desktop release and verifies its manifest. It
// trusts nothing it reads until the manifest verifies: every URL is built
// here from a version that parsed and a filename that was signed.
type feed struct {
	ep           endpoints
	client       *http.Client
	keys         map[string]ed25519.PublicKey
	goos, goarch string
}

// latest returns the newest release on channel that is newer than current,
// or nil when current is the newest.
func (f *feed) latest(ctx context.Context, channel string, current version) (*found, error) {
	tags, err := f.tags(ctx)
	if err != nil {
		return nil, err
	}
	var best string
	var bestV version
	for _, t := range tags {
		v, _ := parseVersion(t)
		if channel != "prerelease" && v.prerelease() {
			continue
		}
		if best == "" || v.compare(bestV) > 0 {
			best, bestV = t, v
		}
	}
	if best == "" {
		return nil, fail(KindRelease, "no Sonde Desktop release on the %s channel", channel)
	}
	if bestV.compare(current) <= 0 {
		return nil, nil
	}
	m, err := f.manifest(ctx, best)
	if err != nil {
		return nil, err
	}
	if err := manifest.Verify(m, f.keys); err != nil {
		return nil, err
	}
	if m.Version != best {
		return nil, &Error{Kind: KindVerification, Err: fmt.Errorf("%w: the manifest of %s names %s", manifest.ErrVerification, best, m.Version)}
	}
	a, err := manifest.PickFor(m, f.goos, f.goarch)
	if err != nil {
		return nil, &Error{Kind: KindRelease, Err: err}
	}
	return &found{manifest: m, artifact: a}, nil
}

// tags lists the versions of every desktop/v* tag, following GitHub's
// pagination.
func (f *feed) tags(ctx context.Context) ([]string, error) {
	next := at(f.ep.api, "repos", repoOwner, repoName, "git", "matching-refs", "tags", "desktop", "v") + "?per_page=100"
	var out []string
	for page := 0; next != ""; page++ {
		if page == maxRefPages {
			return nil, fail(KindNetwork, "GitHub listed more than %d pages of tags", maxRefPages)
		}
		resp, err := f.get(ctx, next, true)
		if err != nil {
			return nil, err
		}
		var refs []struct {
			Ref string `json:"ref"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, maxRefsPage)).Decode(&refs)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fail(KindNetwork, "GitHub's tag list: %v", err)
		}
		for _, r := range refs {
			v, ok := strings.CutPrefix(r.Ref, "refs/tags/desktop/v")
			if _, valid := parseVersion(v); ok && valid {
				out = append(out, v)
			}
		}
		next = f.nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

// nextPage is the rel="next" URL of a Link header, when it stays on the
// API host.
func (f *feed) nextPage(link string) string {
	for part := range strings.SplitSeq(link, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		u, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
		if err == nil && u.Scheme == f.ep.api.Scheme && u.Host == f.ep.api.Host {
			return u.String()
		}
	}
	return ""
}

// manifest fetches release v's manifest. A missing one is an error: the
// release is incomplete, which is never "up to date".
func (f *feed) manifest(ctx context.Context, v string) (manifest.Manifest, error) {
	name := "Sonde-Desktop-" + v + ".update.json"
	resp, err := f.get(ctx, releaseFile(f.ep, v, name), false)
	if err != nil {
		return manifest.Manifest{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxManifest+1))
	if err != nil {
		return manifest.Manifest{}, fail(KindNetwork, "%s: %v", name, err)
	}
	if len(b) > maxManifest {
		return manifest.Manifest{}, fail(KindRelease, "%s is larger than %d bytes", name, maxManifest)
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return manifest.Manifest{}, &Error{Kind: KindVerification, Err: fmt.Errorf("%w: %w", manifest.ErrVerification, err)}
	}
	return m, nil
}

// releaseFile is the download URL of a release's file.
func releaseFile(ep endpoints, v, name string) string {
	return at(ep.download, repoOwner, repoName, "releases", "download", "desktop", "v"+v, name)
}

// get sends a GET; a status other than 200 is an error (404: the release
// lacks the file).
func (f *feed) get(ctx context.Context, u string, api bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if !f.ep.allowed(req.URL) {
		return nil, fail(KindNetwork, "refused to request %s://%s", req.URL.Scheme, req.URL.Host)
	}
	req.Header.Set("User-Agent", "Sonde-Desktop")
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, &Error{Kind: KindNetwork, Err: err}
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		if !api {
			return nil, fail(KindRelease, "the release has no %s yet", req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:])
		}
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fail(KindNetwork, "GitHub's rate limit (HTTP %d): try again later", resp.StatusCode)
	}
	return nil, fail(KindNetwork, "%s: HTTP %d", req.URL.Host, resp.StatusCode)
}
