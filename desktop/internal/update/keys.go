// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"crypto/ed25519"
	"embed"
	"errors"
	"io/fs"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

// keyFiles are the pinned public keys (keys/*.pub).
//
//go:embed keys
var keyFiles embed.FS

// testKey, set with -X (base64 of an ed25519 public key), replaces the
// pinned keys for a -dev build only: a test release server's manifests are
// signed with a throwaway key. A release build ignores it.
var testKey string

// pinnedKeys is the key set manifests are verified against for an app of
// version v.
func pinnedKeys(v string) (map[string]ed25519.PublicKey, error) {
	if testKey != "" && devBuild(v) {
		k, err := manifest.ParsePublicKey([]byte(testKey))
		if err != nil {
			return nil, err
		}
		return map[string]ed25519.PublicKey{"test": k}, nil
	}
	sub, err := fs.Sub(keyFiles, "keys")
	if err != nil {
		return nil, err
	}
	keys, err := manifest.LoadKeys(sub)
	if errors.Is(err, manifest.ErrNoKeys) {
		return nil, errors.New("this build pins no update key")
	}
	return keys, err
}

// devBuild reports whether v's prerelease part starts with "dev"
// (0.2.0-dev, 0.2.0-dev.3).
func devBuild(v string) bool {
	p, ok := parseVersion(v)
	return ok && p.prerelease() && p.pre[0] == "dev"
}
