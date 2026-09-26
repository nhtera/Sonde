// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package config builds the run configuration a command line assembles
// before an entry ever runs: typed variables and secrets from the command
// line, properties files and environment variables, plus the small set of
// options read from a config file. It has no knowledge of cobra flags or
// the engine; internal/cli composes its results into engine.Options,
// applying the command line's own precedence on top.
package config
