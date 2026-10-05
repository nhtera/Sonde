// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"bufio"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// InstallerDeps are what an installer needs from the window app.
type InstallerDeps struct {
	// Quit asks the app to quit (the close guard already lets it through).
	Quit func()
	// Restart is the Wails updater's Restart: on macOS it starts the helper
	// that swaps the bundle, then quits.
	Restart func(ctx context.Context) error
}

// Reasons an installer gives when this copy cannot update itself in place;
// the page offers the release page with them.
const (
	reasonUnsupported  = "Sonde cannot install updates on this system: download them from the release page."
	reasonTranslocated = "Move Sonde to the Applications folder to install updates in place."
	reasonVolume       = "Sonde is on a different volume from the temporary folder, so it cannot install updates in place."
	reasonNotOwned     = "Sonde was installed by another user or by an administrator, so it cannot replace itself."
	reasonNotInstalled = "This copy of Sonde was not installed with its installer, so it cannot replace itself."
	reasonNotAppImage  = "Sonde is not running from an AppImage, so it cannot replace itself."
	reasonShared       = "The AppImage's folder is shared or read-only, so Sonde cannot replace it."
)

// rehash checks the bytes r reads against the signed size and SHA-512: the
// staged file, read again through the handle that is then installed.
func rehash(r io.Reader, a Record) error {
	h := sha512.New()
	n, err := io.Copy(h, io.LimitReader(r, a.Artifact.Size+1))
	if err != nil {
		return err
	}
	if n != a.Artifact.Size || hex.EncodeToString(h.Sum(nil)) != a.Artifact.SHA512 {
		return &Error{Kind: KindVerification, Err: fmt.Errorf("%s changed after it was downloaded", a.Artifact.Filename)}
	}
	return nil
}

// translocated reports whether macOS runs the app from a randomized
// read-only copy (an app opened from the disk image or Downloads).
func translocated(bundle string) bool {
	return strings.Contains(bundle, "/AppTranslocation/")
}

// bundleOf is the .app folder that holds the executable exe, or "" (macOS
// paths, handled alike on any OS for the tests).
func bundleOf(exe string) string {
	for d := path.Dir(exe); d != path.Dir(d); d = path.Dir(d) {
		if strings.HasSuffix(d, ".app") {
			return d
		}
	}
	return ""
}

// ownedTree walks root and returns the first entry that mine refuses (not
// owned by the user, or not writable by them), or "" when every entry is
// the user's. A bundle with one file the user cannot replace is never
// half-deleted by a swap.
func ownedTree(fsys fs.FS, mine func(fs.FileInfo) bool) (string, error) {
	var bad string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !mine(fi) {
			bad = p
			return fs.SkipAll
		}
		return nil
	})
	return bad, err
}

// installedHere decides from the machine-wide Uninstall key whether the
// running exe is the installed one: its InstallLocation, or for a 0.1.0
// install the folder of its DisplayIcon (the exe), equals exeDir. get
// reads a value of the HKLM key (64-bit view), "" when missing.
func installedHere(get func(name string) string, exeDir string) bool {
	dir := get("InstallLocation")
	if dir == "" {
		if icon := strings.Trim(get("DisplayIcon"), `"`); icon != "" {
			dir = winDir(icon)
		}
	}
	return dir != "" && samePathFold(winClean(dir), winClean(exeDir))
}

// winDir and winClean handle Windows paths on any OS (the logic is tested
// everywhere).
func winDir(p string) string {
	p = winClean(p)
	if i := strings.LastIndex(p, `\`); i > 0 {
		return p[:i]
	}
	return p
}

func winClean(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	return strings.TrimRight(p, `\`)
}

func samePathFold(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// mount is one line of /proc/self/mountinfo.
type mount struct {
	point  string // where it is mounted
	fstype string
	source string
}

// parseMountinfo reads /proc/self/mountinfo (proc(5)): the mount point is
// field 5, then optional fields up to "-", then the type and the source.
func parseMountinfo(r io.Reader) ([]mount, error) {
	var out []mount
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if len(f) < 5 || sep < 0 || sep+2 >= len(f) {
			continue
		}
		out = append(out, mount{point: unescapeMount(f[4]), fstype: f[sep+1], source: unescapeMount(f[sep+2])})
	}
	return out, sc.Err()
}

// unescapeMount undoes mountinfo's octal escapes (\040 for a space).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// appImageMount reports how APPDIR is mounted: whether a FUSE mount is
// there at all, and whether its source names the AppImage by its path, or
// only by its file name (as some runtimes record it).
func appImageMount(mounts []mount, appDir, appImage string) (fuse, byPath, byName bool) {
	for _, m := range mounts {
		if filepath.Clean(m.point) != filepath.Clean(appDir) || !strings.HasPrefix(m.fstype, "fuse") {
			continue
		}
		fuse = true
		switch {
		case filepath.Clean(m.source) == filepath.Clean(appImage):
			byPath = true
		case !strings.Contains(m.source, "/") && m.source == filepath.Base(appImage):
			byName = true
		}
	}
	return fuse, byPath, byName
}

// insideDir reports whether p is dir or below it.
func insideDir(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// privateDir reports whether a folder's mode lets only its owner write.
func privateDir(mode fs.FileMode) bool { return mode.IsDir() && mode.Perm()&0o022 == 0 }
