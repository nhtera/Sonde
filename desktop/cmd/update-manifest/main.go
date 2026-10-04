// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Command update-manifest makes and checks the signed update manifest of a
// desktop/v* release (Sonde-Desktop-<v>.update.json, see
// internal/update/manifest):
//
//	update-manifest genkey -id k1 -out DIR
//	update-manifest sign -version V -key FILE -keyid k1 -notes FILE -out FILE UPDATE-FILES
//	update-manifest verify -keys DIR -manifest FILE [-version V] [-dir DIR]
//
// The private key is read from a file only, never from an argument. sign
// takes exactly the release's update files, named as the release workflow
// names them; verify checks the signature against a key set and every file
// in DIR against its signed size and SHA-512.
package main

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "update-manifest:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: update-manifest genkey|sign|verify [flags]")
	}
	switch args[0] {
	case "genkey":
		return genkey(args[1:], stdout)
	case "sign":
		return sign(args[1:], stdout)
	case "verify":
		return verify(args[1:], stdout)
	}
	return fmt.Errorf("unknown command %q: genkey, sign or verify", args[0])
}

// target is where one update file goes in the manifest.
type target struct{ platform, arch string }

// updateFiles is every update file of release v, by name: the names
// release-desktop.yml gives them. Any other file is refused.
func updateFiles(v string) map[string]target {
	p := "Sonde-Desktop-" + v + "-"
	return map[string]target{
		p + "macos-universal.zip":     {"darwin", "universal"},
		p + "windows-amd64-setup.exe": {"windows", "amd64"},
		p + "windows-arm64-setup.exe": {"windows", "arm64"},
		p + "linux-x86_64.AppImage":   {"linux", "amd64"},
	}
}

// manifestName is the release asset's name for version v.
func manifestName(v string) string { return "Sonde-Desktop-" + v + ".update.json" }

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func genkey(args []string, stdout io.Writer) error {
	fs := newFlags("genkey")
	id := fs.String("id", "", "key id (k1, k2, ...)")
	out := fs.String("out", "", "folder for <id>.key and <id>.pub")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *out == "" || fs.NArg() > 0 {
		return errors.New("usage: genkey -id ID -out DIR")
	}
	// The private key is made offline, never next to a repository where a
	// `git add` publishes it.
	if repo, err := gitWorktree(*out); err != nil {
		return err
	} else if repo != "" {
		return fmt.Errorf("-out %s is inside the git worktree %s: write the keys to an offline folder, then copy the .pub files", *out, repo)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	// A key file never replaces another: losing a key strands every install.
	keyPath, pubPath := filepath.Join(*out, *id+".key"), filepath.Join(*out, *id+".pub")
	enc := base64.StdEncoding.EncodeToString
	if err := create(keyPath, enc(priv)+"\n", 0o600); err != nil {
		return err
	}
	if err := create(pubPath, enc(pub)+"\n", 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (private: keep offline) and %s (copy to internal/update/keys)\n", keyPath, pubPath)
	return nil
}

// gitWorktree returns the worktree that holds dir (a folder with .git, at
// dir or above), or "".
func gitWorktree(dir string) (string, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", nil
		}
		d = parent
	}
}

func create(path, data string, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.WriteString(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func sign(args []string, stdout io.Writer) error {
	fs := newFlags("sign")
	version := fs.String("version", "", "the release's version (no v)")
	keyFile := fs.String("key", "", "file holding the base64 ed25519 private key")
	keyID := fs.String("keyid", "", "the key's id (k1, k2, ...)")
	notesFile := fs.String("notes", "", "release notes file (signed, shown in the app)")
	out := fs.String("out", "", "manifest file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *keyFile == "" || *keyID == "" || *notesFile == "" || *out == "" {
		return errors.New("usage: sign -version V -key FILE -keyid ID -notes FILE -out FILE UPDATE-FILES")
	}
	if filepath.Base(*out) != manifestName(*version) {
		return fmt.Errorf("-out: the manifest of %s is named %s", *version, manifestName(*version))
	}
	b, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	key, err := manifest.ParsePrivateKey(b)
	if err != nil {
		return fmt.Errorf("%s: %w", *keyFile, err)
	}
	notes, err := os.ReadFile(*notesFile)
	if err != nil {
		return err
	}

	want := updateFiles(*version)
	m := manifest.Manifest{Schema: manifest.Schema, Version: *version, Notes: string(notes)}
	for _, f := range fs.Args() {
		name := filepath.Base(f)
		t, ok := want[name]
		if !ok {
			return fmt.Errorf("%s is not an update file of %s", name, *version)
		}
		delete(want, name)
		size, sum, err := hashFile(f)
		if err != nil {
			return err
		}
		m.Artifacts = append(m.Artifacts, manifest.Artifact{Platform: t.platform, Arch: t.arch, Filename: name, Size: size, SHA512: sum})
	}
	if len(want) > 0 {
		var missing []string
		for name := range want {
			missing = append(missing, name)
		}
		slices.Sort(missing)
		return fmt.Errorf("missing update files: %v", missing)
	}
	if m, err = manifest.Sign(m, key, *keyID); err != nil {
		return err
	}
	// Signed and checked with the key's own public half before it is written.
	if err := manifest.Verify(m, map[string]ed25519.PublicKey{*keyID: key.Public().(ed25519.PublicKey)}); err != nil {
		return err
	}
	j, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(j, '\n'), 0o600); err != nil { //nolint:gosec // G703: the release workflow's own path
		return err
	}
	fmt.Fprintf(stdout, "signed %s with %s: %d update files\n", *out, *keyID, len(m.Artifacts))
	return nil
}

func verify(args []string, stdout io.Writer) error {
	fs := newFlags("verify")
	keysDir := fs.String("keys", "", "folder of the pinned <id>.pub keys")
	file := fs.String("manifest", "", "the manifest file")
	version := fs.String("version", "", "the version it must name (optional)")
	dir := fs.String("dir", "", "folder of the update files (default: the manifest's)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keysDir == "" || *file == "" || fs.NArg() > 0 {
		return errors.New("usage: verify -keys DIR -manifest FILE [-version V] [-dir DIR]")
	}
	if *dir == "" {
		*dir = filepath.Dir(*file)
	}
	keys, err := manifest.LoadKeys(os.DirFS(*keysDir))
	if err != nil {
		return fmt.Errorf("%s: %w", *keysDir, err)
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return err
	}
	if err := manifest.Verify(m, keys); err != nil {
		return err
	}
	if *version != "" && m.Version != *version {
		return fmt.Errorf("the manifest names %s, want %s", m.Version, *version)
	}
	if filepath.Base(*file) != manifestName(m.Version) {
		return fmt.Errorf("%s: the manifest of %s is named %s", *file, m.Version, manifestName(m.Version))
	}
	want := updateFiles(m.Version)
	if len(m.Artifacts) != len(want) {
		return fmt.Errorf("the manifest lists %d update files, want %d", len(m.Artifacts), len(want))
	}
	for _, a := range m.Artifacts {
		if t, ok := want[a.Filename]; !ok || t != (target{a.Platform, a.Arch}) {
			return fmt.Errorf("%s is not the %s/%s update file of %s", a.Filename, a.Platform, a.Arch, m.Version)
		}
		size, sum, err := hashFile(filepath.Join(*dir, a.Filename))
		if err != nil {
			return err
		}
		if size != a.Size || sum != a.SHA512 {
			return fmt.Errorf("%s: %d bytes, SHA-512 %s; the manifest signed %d bytes, %s", a.Filename, size, sum, a.Size, a.SHA512)
		}
	}
	fmt.Fprintf(stdout, "ok %s: signed by %s, %d update files match\n", filepath.Base(*file), m.KeyID, len(m.Artifacts))
	return nil
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha512.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
