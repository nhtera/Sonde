// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// provider is the Wails updater's only source. It never looks anything up:
// Check hands back the release the feed already verified, with the signed
// SHA-512 for Wails to check while downloading, and Download fetches the
// signed file, never more than its signed size.
type provider struct {
	ep     endpoints
	client *http.Client

	mu  sync.Mutex
	rel *found
	// progress, when set, sees every write of a download, in order, on the
	// downloading goroutine.
	progress func(written, total int64)
}

// providerName names the provider to Wails.
const providerName = "sonde"

func (p *provider) Name() string { return providerName }

// offer sets the verified release the next Check returns.
func (p *provider) offer(f *found, progress func(written, total int64)) {
	p.mu.Lock()
	p.rel, p.progress = f, progress
	p.mu.Unlock()
}

func (p *provider) current() (*found, func(int64, int64)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rel, p.progress
}

// Check returns the verified release; it makes no request.
func (p *provider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	f, _ := p.current()
	if f == nil {
		return nil, errors.New("no verified release to install")
	}
	digest, err := hex.DecodeString(f.artifact.SHA512)
	if err != nil {
		return nil, err
	}
	return &updater.Release{
		Version: f.manifest.Version,
		Notes:   f.manifest.Notes,
		Artifact: updater.Artifact{
			Filename: f.artifact.Filename,
			Size:     f.artifact.Size,
			Platform: f.artifact.Platform,
			Arch:     f.artifact.Arch,
		},
		Verification: &updater.Verification{DigestAlgo: "sha512", Digest: digest},
	}, nil
}

// Download streams the update file of the verified release to dst: from a
// URL built here, cut off past the signed size, and refused short.
func (p *provider) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	f, progress := p.current()
	if f == nil || r.Version != f.manifest.Version || r.Artifact.Filename != f.artifact.Filename {
		return errors.New("the release to download is not the verified one")
	}
	size, name := f.artifact.Size, f.artifact.Filename
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseFile(p.ep, f.manifest.Version, name), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Sonde-Desktop")
	resp, err := p.client.Do(req)
	if err != nil {
		return &Error{Kind: KindNetwork, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(KindNetwork, "%s: HTTP %d", name, resp.StatusCode)
	}
	var written int64
	buf := make([]byte, 64<<10)
	// One byte past the signed size is enough to refuse it.
	body := io.LimitReader(resp.Body, size+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if written+int64(n) > size {
				return fail(KindVerification, "%s is larger than its signed %d bytes", name, size)
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			onProgress(written, size)
			if progress != nil {
				progress(written, size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return &Error{Kind: KindNetwork, Err: fmt.Errorf("%s: %w", name, rerr)}
		}
	}
	if written != size {
		return fail(KindVerification, "%s has %d bytes, signed %d", name, written, size)
	}
	return nil
}
