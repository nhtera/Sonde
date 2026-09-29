// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package osfile moves files to the system trash and shows them in the
// file manager. Programs are started with argument lists, never a shell.
// The window app only: server mode registers neither.
package osfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Reveal shows path in the system file manager.
func Reveal(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", "-R", "--", path)
	case "windows":
		cmd = exec.CommandContext(ctx, "explorer", "/select,"+path) //nolint:gosec // G204: an argument, not a shell
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", filepath.Dir(path)) //nolint:gosec // G204: an argument, not a shell
	}
	return cmd.Start()
}

// Trash moves path (a file or folder) to the user's trash.
func Trash(ctx context.Context, path string) error {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return moveInto(filepath.Join(home, ".Trash"), path)
	case "windows":
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		// The path reaches the script through an environment variable:
		// PowerShell joins every argument after -Command into the script
		// text, so a file name must never be one of them.
		script := `$p = $env:SONDE_TRASH_PATH; Add-Type -AssemblyName Microsoft.VisualBasic; ` +
			`if (Test-Path -LiteralPath $p -PathType Container) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin') } ` +
			`else { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin') }`
		cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "SONDE_TRASH_PATH="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("trash: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return trashFreedesktop(path)
	}
}

// trashFreedesktop follows the freedesktop.org trash spec: the item moves
// to $XDG_DATA_HOME/Trash/files with a .trashinfo record.
func trashFreedesktop(path string) error {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		data = filepath.Join(home, ".local", "share")
	}
	trash := filepath.Join(data, "Trash")
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	if err := os.MkdirAll(filepath.Join(trash, "info"), 0o700); err != nil {
		return err
	}
	name, err := freeName(filepath.Join(trash, "files"), filepath.Base(path))
	if err != nil {
		return err
	}
	info := "[Trash Info]\nPath=" + escapePath(path) + "\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
	infoPath := filepath.Join(trash, "info", name+".trashinfo")
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	if err := os.WriteFile(infoPath, []byte(info), 0o600); err != nil {
		return err
	}
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	if err := os.Rename(path, filepath.Join(trash, "files", name)); err != nil {
		_ = os.Remove(infoPath) //nolint:forbidigo // undo the record above
		return err
	}
	return nil
}

// moveInto moves path into dir under a free name.
func moveInto(dir, path string) error {
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name, err := freeName(dir, filepath.Base(path))
	if err != nil {
		return err
	}
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	return os.Rename(path, filepath.Join(dir, name))
}

// freeName returns base, or "base 2", "base 3"… (before the extension),
// whichever does not exist in dir.
func freeName(dir, base string) (string, error) {
	//nolint:forbidigo,gosec // the user's trash, outside any project Root; the path is a project file
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		name := base
		if i > 1 {
			name = stem + " " + strconv.Itoa(i) + ext
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) { //nolint:gosec // G703: a name in the trash folder
			return name, nil
		}
	}
	return "", errors.New("trash: no free name")
}

// escapePath percent-encodes a path for a .trashinfo record.
func escapePath(p string) string {
	var b strings.Builder
	for _, c := range []byte(p) {
		if c == '/' || c == '-' || c == '_' || c == '.' || c == '~' ||
			'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
