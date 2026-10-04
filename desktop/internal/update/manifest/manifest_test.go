// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func digest(c byte) string { return strings.Repeat(string(c), 128) }

func sample() Manifest {
	return Manifest{Schema: Schema, Version: "0.2.1", Notes: "Sonde Desktop 0.2.1", Artifacts: []Artifact{
		{"darwin", "universal", "Sonde-Desktop-0.2.1-macos-universal.zip", 27000000, digest('a')},
		{"windows", "amd64", "Sonde-Desktop-0.2.1-windows-amd64-setup.exe", 13000000, digest('b')},
		{"windows", "arm64", "Sonde-Desktop-0.2.1-windows-arm64-setup.exe", 12000000, digest('c')},
		{"linux", "amd64", "Sonde-Desktop-0.2.1-linux-x86_64.AppImage", 85000000, digest('d')},
	}}
}

func keypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestSignVerify(t *testing.T) {
	pub, priv := keypair(t)
	other, _ := keypair(t)
	keys := map[string]ed25519.PublicKey{"k1": pub}
	signed, err := Sign(sample(), priv, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(signed, keys); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	// Artifact order is not signed: the canonical lines are sorted.
	reordered := signed
	reordered.Artifacts = append([]Artifact{signed.Artifacts[3]}, signed.Artifacts[:3]...)
	if err := Verify(reordered, keys); err != nil {
		t.Fatalf("reordered artifacts: %v", err)
	}

	cases := map[string]func(m *Manifest){
		"version":  func(m *Manifest) { m.Version = "0.2.2" },
		"notes":    func(m *Manifest) { m.Notes += "!" },
		"filename": func(m *Manifest) { m.Artifacts[0].Filename = "Sonde-Desktop-0.2.1-macos-universal2.zip" },
		"size":     func(m *Manifest) { m.Artifacts[1].Size++ },
		"digest":   func(m *Manifest) { m.Artifacts[2].SHA512 = digest('e') },
		"platform": func(m *Manifest) { m.Artifacts[3].Platform = "freebsd" },
		"swapped": func(m *Manifest) {
			// The arm64 installer advertised as the amd64 one, and back.
			a, b := &m.Artifacts[1], &m.Artifacts[2]
			a.Filename, b.Filename = b.Filename, a.Filename
			a.Size, b.Size = b.Size, a.Size
			a.SHA512, b.SHA512 = b.SHA512, a.SHA512
		},
		"dropped":      func(m *Manifest) { m.Artifacts = m.Artifacts[:3] },
		"unknown key":  func(m *Manifest) { m.KeyID = "k9" },
		"no signature": func(m *Manifest) { m.Signature = "" },
		"bad base64":   func(m *Manifest) { m.Signature = "%%%" },
		"schema":       func(m *Manifest) { m.Schema = 2 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := signed
			m.Artifacts = append([]Artifact(nil), signed.Artifacts...)
			change(&m)
			if err := Verify(m, keys); !errors.Is(err, ErrVerification) {
				t.Fatalf("Verify = %v, want a verification error", err)
			}
		})
	}
	t.Run("wrong key", func(t *testing.T) {
		if err := Verify(signed, map[string]ed25519.PublicKey{"k1": other}); !errors.Is(err, ErrVerification) {
			t.Fatalf("Verify = %v, want a verification error", err)
		}
	})
	t.Run("standby key signs too", func(t *testing.T) {
		pub2, priv2 := keypair(t)
		m, err := Sign(sample(), priv2, "k2")
		if err != nil {
			t.Fatal(err)
		}
		if err := Verify(m, map[string]ed25519.PublicKey{"k1": pub, "k2": pub2}); err != nil {
			t.Fatalf("k2 signature with both keys pinned: %v", err)
		}
	})
}

func TestCanonicalRefusesAmbiguity(t *testing.T) {
	cases := map[string]func(m *Manifest){
		"space in filename":   func(m *Manifest) { m.Artifacts[0].Filename = "a b.zip" },
		"dot dot filename":    func(m *Manifest) { m.Artifacts[0].Filename = ".." },
		"hidden filename":     func(m *Manifest) { m.Artifacts[0].Filename = ".Sonde.zip" },
		"newline in arch":     func(m *Manifest) { m.Artifacts[0].Arch = "universal\nlinux" },
		"uppercase digest":    func(m *Manifest) { m.Artifacts[0].SHA512 = strings.Repeat("A", 128) },
		"short digest":        func(m *Manifest) { m.Artifacts[0].SHA512 = digest('a')[:64] },
		"zero size":           func(m *Manifest) { m.Artifacts[0].Size = 0 },
		"twice the target":    func(m *Manifest) { m.Artifacts[2].Arch = "amd64" },
		"twice the file":      func(m *Manifest) { m.Artifacts[2].Filename = m.Artifacts[1].Filename },
		"version with v":      func(m *Manifest) { m.Version = "v0.2.1" },
		"version with spaces": func(m *Manifest) { m.Version = "0.2.1\nlinux" },
		"no artifact":         func(m *Manifest) { m.Artifacts = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := sample()
			change(&m)
			if _, err := Canonical(m); err == nil {
				t.Fatal("Canonical accepted it")
			}
		})
	}
}

func TestCanonicalBytes(t *testing.T) {
	m := Manifest{Schema: Schema, Version: "1.0.0-rc.1", Notes: "", Artifacts: []Artifact{
		{"windows", "amd64", "w.exe", 2, digest('b')},
		{"darwin", "universal", "m.zip", 1, digest('a')},
	}}
	got, err := Canonical(m)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-512 of the empty string.
	empty := "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	want := "sonde-desktop-update/1\n1.0.0-rc.1\n" + empty + "\n" +
		"darwin universal m.zip 1 " + digest('a') + "\n" +
		"windows amd64 w.exe 2 " + digest('b') + "\n"
	if string(got) != want {
		t.Fatalf("Canonical =\n%s\nwant\n%s", got, want)
	}
}

func TestPickFor(t *testing.T) {
	m := sample()
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "Sonde-Desktop-0.2.1-macos-universal.zip"},
		{"darwin", "amd64", "Sonde-Desktop-0.2.1-macos-universal.zip"},
		{"windows", "amd64", "Sonde-Desktop-0.2.1-windows-amd64-setup.exe"},
		{"windows", "arm64", "Sonde-Desktop-0.2.1-windows-arm64-setup.exe"},
		{"linux", "amd64", "Sonde-Desktop-0.2.1-linux-x86_64.AppImage"},
	} {
		a, err := PickFor(m, c.goos, c.goarch)
		if err != nil || a.Filename != c.want {
			t.Errorf("PickFor(%s/%s) = %q, %v; want %q", c.goos, c.goarch, a.Filename, err, c.want)
		}
	}
	if _, err := PickFor(m, "linux", "arm64"); err == nil {
		t.Error("PickFor(linux/arm64): no such build, want an error")
	}
}

func TestKeys(t *testing.T) {
	pub, priv := keypair(t)
	enc := base64.StdEncoding.EncodeToString
	got, err := ParsePrivateKey([]byte(enc(priv) + "\n"))
	if err != nil || !got.Equal(priv) {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	bad := append(ed25519.PrivateKey(nil), priv...)
	bad[63] ^= 1
	if _, err := ParsePrivateKey([]byte(enc(bad))); err == nil {
		t.Error("ParsePrivateKey accepted a key whose public half is not the seed's")
	}
	if _, err := ParsePrivateKey([]byte(enc(priv.Seed()))); err == nil {
		t.Error("ParsePrivateKey accepted a bare seed")
	}

	keys, err := LoadKeys(fstest.MapFS{
		"k1.pub":    {Data: []byte(enc(pub) + "\n")},
		"README.md": {Data: []byte("not a key")},
	})
	if err != nil || len(keys) != 1 || !keys["k1"].Equal(pub) {
		t.Fatalf("LoadKeys = %v, %v", keys, err)
	}
	if _, err := LoadKeys(fstest.MapFS{"k1.pub": {Data: []byte("short")}}); err == nil {
		t.Error("LoadKeys accepted a malformed key")
	}
	if _, err := LoadKeys(fstest.MapFS{"K 1.pub": {Data: []byte(enc(pub))}}); err == nil {
		t.Error("LoadKeys accepted a key id with a space")
	}
	if _, err := LoadKeys(fstest.MapFS{}); err == nil {
		t.Error("LoadKeys accepted an empty key set")
	}
}

func TestParse(t *testing.T) {
	m, err := Parse([]byte(`{"schema":1,"version":"0.2.1","notes":"n","artifacts":[{"platform":"linux","arch":"amd64","filename":"f","size":1,"sha512":"x"}],"keyId":"k1","signature":"s"}`))
	if err != nil || m.Version != "0.2.1" || len(m.Artifacts) != 1 || m.Artifacts[0].Size != 1 || m.KeyID != "k1" {
		t.Fatalf("Parse = %+v, %v", m, err)
	}
	for name, in := range map[string]string{
		"unknown field": `{"schema":1,"extra":true}`,
		"trailing data": `{"schema":1} {}`,
		"not JSON":      `<html>`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: Parse accepted %s", name, in)
		}
	}
}
