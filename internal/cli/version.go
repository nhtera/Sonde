// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/httpx"
)

// Build metadata injected by release builds:
//
//	-ldflags "-X github.com/nhtera/sonde/internal/cli.version=... -X ...commit=... -X ...date=..."
var (
	version string
	commit  string
	date    string
)

// buildInfo describes the running binary.
type buildInfo struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
}

// currentBuildInfo combines ldflags values with the Go build info embedded
// by the toolchain, so `go install` and plain `go build` binaries still
// report a module version and VCS revision.
func currentBuildInfo() buildInfo {
	info, _ := debug.ReadBuildInfo()
	return resolveBuildInfo(version, commit, date, info)
}

// resolveBuildInfo prefers ldflags values, falls back to the embedded build
// info (info may be nil), then to placeholders.
func resolveBuildInfo(ldVersion, ldCommit, ldDate string, info *debug.BuildInfo) buildInfo {
	bi := buildInfo{Version: ldVersion, Commit: ldCommit, Date: ldDate, GoVersion: runtime.Version()}
	if info != nil {
		if bi.Version == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			bi.Version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && bi.Commit == "":
				bi.Commit = s.Value
			case s.Key == "vcs.time" && bi.Date == "":
				bi.Date = s.Value
			}
		}
	}
	if bi.Version == "" {
		bi.Version = "dev"
	}
	if bi.Commit == "" {
		bi.Commit = "unknown"
	}
	if bi.Date == "" {
		bi.Date = "unknown"
	}
	return bi
}

// writeVersion prints the version block. Its "Features:" line lists the
// optional transport features this build implements (httpx.Features), so
// scripts can probe for one with `--version | grep Features`.
func writeVersion(w io.Writer, bi buildInfo) error {
	_, err := fmt.Fprintf(w, "sonde %s\ncommit: %s\nbuilt: %s\ngo: %s\nFeatures: %s\n",
		bi.Version, bi.Commit, bi.Date, bi.GoVersion, strings.Join(httpx.Features(), " "))
	return err
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, build date and Go version",
		Args:  cobra.NoArgs,
		RunE: typed(func(cmd *cobra.Command, _ []string) error {
			return writeVersion(cmd.OutOrStdout(), currentBuildInfo())
		}),
	}
}
