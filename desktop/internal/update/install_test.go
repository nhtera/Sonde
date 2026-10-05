// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nhtera/sonde/desktop/internal/update/manifest"
)

func TestTranslocationAndBundle(t *testing.T) {
	if !translocated("/private/var/folders/x/T/AppTranslocation/ABCD/d/Sonde.app") || translocated("/Applications/Sonde.app") {
		t.Error("translocated")
	}
	if got := bundleOf("/Applications/Sonde.app/Contents/MacOS/sonde-desktop"); got != "/Applications/Sonde.app" {
		t.Errorf("bundleOf = %q", got)
	}
	if got := bundleOf("/usr/local/bin/sonde-desktop"); got != "" {
		t.Errorf("bundleOf a loose binary = %q", got)
	}
}

// owner is a fake file owner, carried in fstest's Sys.
type owner struct{ uid int }

func TestOwnedTree(t *testing.T) {
	mine := func(fi fs.FileInfo) bool {
		o, ok := fi.Sys().(owner)
		return ok && o.uid == 501 && fi.Mode().Perm()&0o200 != 0
	}
	tree := func(edit func(m fstest.MapFS)) fstest.MapFS {
		m := fstest.MapFS{
			"Contents/Info.plist":           {Mode: 0o644, Sys: owner{501}},
			"Contents/MacOS/sonde-desktop":  {Mode: 0o755, Sys: owner{501}},
			"Contents/Resources/icons.icns": {Mode: 0o644, Sys: owner{501}},
			"Contents/MacOS":                {Mode: fs.ModeDir | 0o755, Sys: owner{501}},
			"Contents/Resources":            {Mode: fs.ModeDir | 0o755, Sys: owner{501}},
			"Contents":                      {Mode: fs.ModeDir | 0o755, Sys: owner{501}},
			".":                             {Mode: fs.ModeDir | 0o755, Sys: owner{501}},
		}
		if edit != nil {
			edit(m)
		}
		return m
	}
	if bad, err := ownedTree(tree(nil), mine); err != nil || bad != "" {
		t.Fatalf("all mine: %q, %v", bad, err)
	}
	if bad, _ := ownedTree(tree(func(m fstest.MapFS) { m["Contents/Resources/icons.icns"].Sys = owner{0} }), mine); bad != "Contents/Resources/icons.icns" {
		t.Errorf("a root-owned file: %q", bad)
	}
	if bad, _ := ownedTree(tree(func(m fstest.MapFS) { m["Contents/MacOS"].Mode = fs.ModeDir | 0o555 }), mine); bad != "Contents/MacOS" {
		t.Errorf("a read-only folder: %q", bad)
	}
}

func TestInstalledHere(t *testing.T) {
	reg := func(vals map[string]string) func(string) string { return func(n string) string { return vals[n] } }
	dir := `C:\Program Files\The Sonde Authors\Sonde`
	cases := []struct {
		name string
		vals map[string]string
		exe  string
		want bool
	}{
		{"InstallLocation", map[string]string{"InstallLocation": dir}, dir, true},
		{"case and a trailing slash", map[string]string{"InstallLocation": strings.ToLower(dir) + `\`}, dir, true},
		{"0.1.0: DisplayIcon", map[string]string{"DisplayIcon": dir + `\Sonde.exe`}, dir, true},
		{"0.1.0: quoted DisplayIcon", map[string]string{"DisplayIcon": `"` + dir + `\Sonde.exe"`}, dir, true},
		{"another folder", map[string]string{"InstallLocation": `D:\Apps\Sonde`}, dir, false},
		{"a portable copy", map[string]string{"InstallLocation": dir}, `C:\Users\u\Downloads`, false},
		{"not installed", map[string]string{}, dir, false},
	}
	for _, c := range cases {
		if got := installedHere(reg(c.vals), c.exe); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// Mountinfo lines of an AppImage (type-2 runtime): with the image's path
// as the source, or only its file name (the runtime Sonde ships records
// that, seen on ubuntu-24.04).
const (
	ubuntuMount = "1251 29 0:62 / /tmp/.mount_SondeaB3xQz ro,nosuid,nodev,relatime shared:651 - fuse.Sonde-Desktop-0.2.0-linux-x86_64.AppImage /home/ana/Apps/Sonde-Desktop-0.2.0-linux-x86_64.AppImage ro,user_id=1000,group_id=1000\n"
	fedoraMount = "771 64 0:58 / /tmp/.mount_Sonde8Kp2Lm ro,nosuid,nodev,relatime - fuse.Sonde.AppImage Sonde.AppImage ro,user_id=1000,group_id=1000\n"
	debianMount = "402 27 0:47 / /tmp/.mount_SondeZ1 ro,nosuid,nodev,relatime shared:250 master:1 - fuse.Sonde\\040Desktop.AppImage /home/ana/My\\040Apps/Sonde\\040Desktop.AppImage ro,user_id=1000,group_id=1000\n"
	otherMounts = "22 1 8:2 / / rw,relatime shared:1 - ext4 /dev/sda2 rw\n25 22 0:23 / /proc rw,nosuid shared:12 - proc proc rw\n"
)

func TestMountinfo(t *testing.T) {
	parse := func(s string) []mount {
		m, err := parseMountinfo(strings.NewReader(s))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	cases := []struct {
		name, info, dir, image string
		fuse, byPath, byName   bool
	}{
		{"a full path", otherMounts + ubuntuMount, "/tmp/.mount_SondeaB3xQz", "/home/ana/Apps/Sonde-Desktop-0.2.0-linux-x86_64.AppImage", true, true, false},
		{"a foreign APPIMAGE", otherMounts + ubuntuMount, "/tmp/.mount_SondeaB3xQz", "/home/ana/Downloads/Other.AppImage", true, false, false},
		{"the file name only (as on ubuntu-24.04)", fedoraMount, "/tmp/.mount_Sonde8Kp2Lm", "/home/ana/Sonde.AppImage", true, false, true},
		{"the file name only, a foreign APPIMAGE", fedoraMount, "/tmp/.mount_Sonde8Kp2Lm", "/home/ana/Other.AppImage", true, false, false},
		{"escaped spaces", debianMount, "/tmp/.mount_SondeZ1", "/home/ana/My Apps/Sonde Desktop.AppImage", true, true, false},
		{"no mount there", otherMounts, "/tmp/.mount_SondeaB3xQz", "/home/ana/Apps/Sonde.AppImage", false, false, false},
		{"not FUSE", otherMounts, "/", "/dev/sda2", false, false, false},
	}
	for _, c := range cases {
		fuse, byPath, byName := appImageMount(parse(c.info), c.dir, c.image)
		if fuse != c.fuse || byPath != c.byPath || byName != c.byName {
			t.Errorf("%s: %v %v %v, want %v %v %v", c.name, fuse, byPath, byName, c.fuse, c.byPath, c.byName)
		}
	}
}

func TestFolderChecks(t *testing.T) {
	if !privateDir(fs.ModeDir|0o755) || privateDir(fs.ModeDir|0o775) || privateDir(fs.ModeDir|0o757) || privateDir(0o644) {
		t.Error("privateDir")
	}
	if !insideDir("/tmp/.mount_x/usr/bin/sonde-desktop", "/tmp/.mount_x") || insideDir("/tmp/.mount_xy/a", "/tmp/.mount_x") || insideDir("/usr/bin/x", "/tmp/.mount_x") {
		t.Error("insideDir")
	}
}

func TestRehash(t *testing.T) {
	body := []byte("the staged installer")
	sum := sha512.Sum512(body)
	r := Record{Version: "0.2.1", Artifact: manifest.Artifact{Filename: "f", Size: int64(len(body)), SHA512: hex.EncodeToString(sum[:])}}
	if err := rehash(bytes.NewReader(body), r); err != nil {
		t.Fatal(err)
	}
	changed := bytes.Clone(body)
	changed[3] ^= 1
	for _, b := range [][]byte{changed, body[:5], append(bytes.Clone(body), '!')} {
		if err := rehash(bytes.NewReader(b), r); kindOf(err) != KindVerification {
			t.Errorf("rehash(%q) = %v, want a verification error", b, err)
		}
	}
}
