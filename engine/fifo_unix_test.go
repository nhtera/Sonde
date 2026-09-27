// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package engine

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestRenderCurlNeverHangsOnFIFO checks that a FileRef body naming a FIFO
// in the file root reports an error for that entry instead of blocking
// forever: RenderCurl (unlike a run) never opens a body file at all, only
// Stat, which cannot block on a FIFO with no writer.
func TestRenderCurlNeverHangsOnFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "ff")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	f := parseEntries(t, "POST http://a/\nfile,ff;\n")

	done := make(chan []CurlEntry, 1)
	go func() {
		entries, err := NewRunner(Options{FileRoot: dir}).RenderCurl(context.Background(), "test.hurl", f)
		if err != nil {
			t.Errorf("RenderCurl: %v", err)
			done <- nil
			return
		}
		done <- entries
	}()
	select {
	case entries := <-done:
		if len(entries) != 1 || entries[0].Err == nil {
			t.Errorf("entries = %+v, want one entry with Err set (not opened, not hung)", entries)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RenderCurl blocked reading a FIFO body")
	}
}

// TestRenderCurlNeverHangsOnFIFOMultipart is the same check for a FIFO
// named by a multipart file field.
func TestRenderCurlNeverHangsOnFIFOMultipart(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "ff")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	f := parseEntries(t, "POST http://a/\n[Multipart]\nfile: file,ff;\n")

	done := make(chan []CurlEntry, 1)
	go func() {
		entries, err := NewRunner(Options{FileRoot: dir}).RenderCurl(context.Background(), "test.hurl", f)
		if err != nil {
			t.Errorf("RenderCurl: %v", err)
			done <- nil
			return
		}
		done <- entries
	}()
	select {
	case entries := <-done:
		if len(entries) != 1 || entries[0].Err == nil {
			t.Errorf("entries = %+v, want one entry with Err set (not opened, not hung)", entries)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RenderCurl blocked reading a FIFO multipart file")
	}
}
