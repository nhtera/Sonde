// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package manifest is the signed update manifest a desktop/v* release
// carries (Sonde-Desktop-<v>.update.json). One ed25519 signature covers the
// version, a digest of the notes and every file's platform, arch, name,
// size and SHA-512, so none of them can change, or move to another
// platform, without the signature failing. The release workflow's
// update-manifest tool signs it; the app verifies it against its pinned
// key set before downloading anything.
package manifest

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Schema is the only manifest schema this package reads and writes.
const Schema = 1

// domain separates these signatures from any other use of the key.
const domain = "sonde-desktop-update/1"

// Manifest is the JSON file. Signature is base64 (standard encoding) of the
// ed25519 signature over Canonical, by the key KeyID names.
type Manifest struct {
	Schema    int        `json:"schema"`
	Version   string     `json:"version"`
	Notes     string     `json:"notes"`
	Artifacts []Artifact `json:"artifacts"`
	KeyID     string     `json:"keyId"`
	Signature string     `json:"signature"`
}

// Artifact is one update file. SHA512 is lowercase hex.
type Artifact struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SHA512   string `json:"sha512"`
}

var (
	// ErrVerification wraps every refusal of a manifest's signature, so a
	// caller can tell it from a network or format error.
	ErrVerification = errors.New("verification failed")

	// A field of a canonical line: no space, no newline, so a line splits
	// back into exactly the fields that were signed; never "." or "..", nor
	// a hidden name.
	token = regexp.MustCompile(`^[A-Za-z0-9_+-][A-Za-z0-9._+-]*$`)
	// Semver 2.0 without leading v: the release workflow's tag names it.
	semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	hex512 = regexp.MustCompile(`^[0-9a-f]{128}$`)
	keyID  = regexp.MustCompile(`^[a-z0-9]+$`)
)

// Canonical returns the signed bytes:
//
//	sonde-desktop-update/1\n<version>\n<sha512 hex of notes>\n
//
// then one line per artifact, sorted: "<platform> <arch> <filename> <size>
// <sha512 hex>\n". It refuses a manifest whose fields could make two
// different manifests sign the same bytes.
func Canonical(m Manifest) ([]byte, error) {
	if m.Schema != Schema {
		return nil, fmt.Errorf("manifest schema %d, want %d", m.Schema, Schema)
	}
	if !semver.MatchString(m.Version) {
		return nil, fmt.Errorf("manifest version %q is not a semantic version", m.Version)
	}
	if len(m.Artifacts) == 0 {
		return nil, errors.New("manifest lists no artifact")
	}
	lines := make([]string, 0, len(m.Artifacts))
	targets := map[string]bool{}
	names := map[string]bool{}
	for _, a := range m.Artifacts {
		for _, f := range []string{a.Platform, a.Arch, a.Filename} {
			if !token.MatchString(f) {
				return nil, fmt.Errorf("manifest artifact field %q: only letters, digits and . _ + -, not first a dot", f)
			}
		}
		if a.Size <= 0 {
			return nil, fmt.Errorf("manifest artifact %s: size %d", a.Filename, a.Size)
		}
		if !hex512.MatchString(a.SHA512) {
			return nil, fmt.Errorf("manifest artifact %s: sha512 is not 128 lowercase hex digits", a.Filename)
		}
		target := a.Platform + " " + a.Arch
		if targets[target] {
			return nil, fmt.Errorf("manifest lists %s/%s twice", a.Platform, a.Arch)
		}
		if names[a.Filename] {
			return nil, fmt.Errorf("manifest lists %s twice", a.Filename)
		}
		targets[target], names[a.Filename] = true, true
		lines = append(lines, fmt.Sprintf("%s %s %s %d %s\n", a.Platform, a.Arch, a.Filename, a.Size, a.SHA512))
	}
	slices.Sort(lines)
	notes := sha512.Sum512([]byte(m.Notes))
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n%s\n%s\n", domain, m.Version, hex.EncodeToString(notes[:]))
	for _, l := range lines {
		b.WriteString(l)
	}
	return b.Bytes(), nil
}

// Parse decodes a manifest file. A field this schema does not know is an
// error: nothing in the file goes unsigned.
func Parse(b []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if dec.More() {
		return Manifest{}, errors.New("manifest: data after the JSON object")
	}
	return m, nil
}

// Sign returns m signed by key under the name id.
func Sign(m Manifest, key ed25519.PrivateKey, id string) (Manifest, error) {
	if !keyID.MatchString(id) {
		return Manifest{}, fmt.Errorf("key id %q: only lowercase letters and digits", id)
	}
	if len(key) != ed25519.PrivateKeySize {
		return Manifest{}, errors.New("not an ed25519 private key")
	}
	m.KeyID, m.Signature = id, ""
	msg, err := Canonical(m)
	if err != nil {
		return Manifest{}, err
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg))
	return m, nil
}

// Verify checks m's signature against the key its KeyID names in keys. Every
// refusal wraps ErrVerification.
func Verify(m Manifest, keys map[string]ed25519.PublicKey) error {
	if m.Signature == "" {
		return fmt.Errorf("%w: the manifest is not signed", ErrVerification)
	}
	pub, ok := keys[m.KeyID]
	if !ok {
		return fmt.Errorf("%w: unknown signing key %q", ErrVerification, m.KeyID)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: malformed signature", ErrVerification)
	}
	msg, err := Canonical(m)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerification, err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("%w: the signature does not match key %q", ErrVerification, m.KeyID)
	}
	return nil
}

// PickFor returns the artifact for an OS and architecture: macOS has one
// universal build; Windows and Linux have one per GOARCH.
func PickFor(m Manifest, goos, goarch string) (Artifact, error) {
	arch := goarch
	if goos == "darwin" {
		arch = "universal"
	}
	for _, a := range m.Artifacts {
		if a.Platform == goos && a.Arch == arch {
			return a, nil
		}
	}
	return Artifact{}, fmt.Errorf("release %s has no update for %s/%s", m.Version, goos, goarch)
}

// ParsePublicKey reads a .pub file: base64 of the 32-byte ed25519 key.
func ParsePublicKey(b []byte) (ed25519.PublicKey, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil, errors.New("not a base64 ed25519 public key")
	}
	return ed25519.PublicKey(k), nil
}

// ParsePrivateKey reads a private key file: base64 of the 64-byte ed25519
// key (seed followed by the public key).
func ParsePrivateKey(b []byte) (ed25519.PrivateKey, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != ed25519.PrivateKeySize {
		return nil, errors.New("not a base64 ed25519 private key")
	}
	key := ed25519.PrivateKey(k)
	// The stored public half must be the seed's: a corrupted file never signs.
	if !bytes.Equal(ed25519.NewKeyFromSeed(key.Seed()), key) {
		return nil, errors.New("not a consistent ed25519 private key")
	}
	return key, nil
}

// LoadKeys reads every <id>.pub in fsys's root into a key set.
func LoadKeys(fsys fs.FS) (map[string]ed25519.PublicKey, error) {
	files, err := fs.Glob(fsys, "*.pub")
	if err != nil {
		return nil, err
	}
	keys := map[string]ed25519.PublicKey{}
	for _, f := range files {
		id := strings.TrimSuffix(path.Base(f), ".pub")
		if !keyID.MatchString(id) {
			return nil, fmt.Errorf("%s: a key id is lowercase letters and digits", f)
		}
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		if keys[id], err = ParsePublicKey(b); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no public key (*.pub)")
	}
	return keys, nil
}
