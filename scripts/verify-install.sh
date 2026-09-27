#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Verifies a published sonde release end to end: brew (macOS/Linux), scoop
# (Windows), `go install`, and a manual download + checksum + cosign bundle
# check. Runs in the release.yml verify-install matrix, and locally against
# any already-published tag.
#
# Usage:   scripts/verify-install.sh [VERSION]
# Version: positional arg, or $SONDE_VERIFY_VERSION, e.g. v1.0.0 or
#          v1.0.0-rc.1. The tap and bucket hold the latest tag, pre-release
#          or not: verify right after publishing.
# Env:
#   SONDE_VERIFY_REPO         "owner/repo" as GitHub spells it (default:
#                             nhtera/Sonde): the release signature names it
#                             with this exact case
#   SONDE_VERIFY_SKIP_BREW    set to skip the brew step
#   SONDE_VERIFY_SKIP_SCOOP   set to skip the scoop step
#   SONDE_VERIFY_SKIP_GO      set to skip the `go install` step
#   SONDE_VERIFY_SKIP_ARCHIVE set to skip the archive/checksum/cosign step
#
# Every step that needs a tool absent on this machine is skipped with a
# clear message rather than failing; a step whose tool IS present but whose
# check fails is a hard failure (exit 1).
set -euo pipefail

VERSION="${1:-${SONDE_VERIFY_VERSION:-}}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 VERSION   (e.g. v1.0.0 or v1.0.0-rc.1)" >&2
  exit 1
fi
REPO="${SONDE_VERIFY_REPO:-nhtera/Sonde}"
# The Go module path is lowercase whatever the repository's case.
MODULE="github.com/nhtera/sonde"
# .Version in the GoReleaser templates: the tag without its leading "v".
PLAIN_VERSION="${VERSION#v}"

fail=0
note() { echo "==> $*"; }
skip() { echo "skip: $*"; }
fail_step() {
  echo "FAIL: $*" >&2
  fail=1
}

# --- platform detection -----------------------------------------------
uname_s="$(uname -s)"
case "$uname_s" in
Darwin) PLATFORM=macos ;;
Linux) PLATFORM=linux ;;
MINGW* | MSYS* | CYGWIN*) PLATFORM=windows ;;
*)
  echo "unsupported platform: $uname_s" >&2
  exit 1
  ;;
esac

uname_m="$(uname -m)"
case "$uname_m" in
x86_64 | amd64) GOARCH=amd64 ;;
arm64 | aarch64) GOARCH=arm64 ;;
*)
  echo "unsupported arch: $uname_m" >&2
  exit 1
  ;;
esac

case "$PLATFORM" in
macos) GOOS=darwin ;;
linux) GOOS=linux ;;
windows) GOOS=windows ;;
esac

note "platform=$PLATFORM arch=$GOARCH version=$VERSION repo=$REPO"

check_version_output() {
  # $1: label, $2: path to the sonde binary (or bare "sonde" on PATH)
  local label="$1" bin="$2" out token
  if ! out="$("$bin" version 2>&1)"; then
    fail_step "$label: '$bin version' failed: $out"
    return
  fi
  # First line is "sonde <version>"; compare that token exactly rather than
  # by substring, so e.g. a "v1.0.0-rc.1" build doesn't pass a "v1.0.0"
  # check just because one contains the other.
  token="$(printf '%s\n' "$out" | head -n1 | awk '{print $2}')"
  if [ "$token" != "$VERSION" ] && [ "$token" != "$PLAIN_VERSION" ]; then
    fail_step "$label: 'sonde version' printed version [$token] (full output: $out), want $VERSION or $PLAIN_VERSION"
    return
  fi
  note "$label: OK ($out)"
}

# --- 1. Homebrew (macOS, Linux) ----------------------------------------
# GitHub's Linux runners ship Homebrew but keep it off PATH.
if [ "$PLATFORM" = linux ] && [ -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"
fi
if [ -n "${SONDE_VERIFY_SKIP_BREW:-}" ]; then
  skip "brew (SONDE_VERIFY_SKIP_BREW set)"
elif [ "$PLATFORM" = windows ]; then
  skip "brew (not applicable on Windows)"
elif ! command -v brew >/dev/null 2>&1; then
  skip "brew (not installed on this machine)"
else
  tap="nhtera/tap"
  note "brew install $tap/sonde"
  if brew install "$tap/sonde"; then
    check_version_output "brew" "$(brew --prefix)/bin/sonde"
    brew uninstall sonde >/dev/null 2>&1 || true
    brew untap "$tap" >/dev/null 2>&1 || true
  else
    fail_step "brew install $tap/sonde failed"
  fi
fi

# --- 2. Scoop (Windows) --------------------------------------------------
if [ -n "${SONDE_VERIFY_SKIP_SCOOP:-}" ]; then
  skip "scoop (SONDE_VERIFY_SKIP_SCOOP set)"
elif [ "$PLATFORM" != windows ]; then
  skip "scoop (not applicable outside Windows)"
elif ! command -v powershell.exe >/dev/null 2>&1 && ! command -v pwsh >/dev/null 2>&1; then
  skip "scoop (no PowerShell found)"
else
  # Bridges to PowerShell: scoop itself is a PowerShell function/shim, not
  # a bash-callable binary. Every call is its own process, so:
  #   - it prepends the scoop shims dir to $env:PATH itself, rather than
  #     relying on the *next* process picking up a registry PATH update
  #     the installer made (a new child process inherits the parent bash
  #     job's environment, not a live re-read of the registry);
  #   - $ErrorActionPreference = 'Stop' turns a scoop cmdlet's
  #     non-terminating error into a terminating one, then the catch
  #     block maps it to a non-zero process exit code, since scoop
  #     failures don't reliably set $LASTEXITCODE on their own.
  pwsh_run() {
    local body="
\$ErrorActionPreference = 'Stop'
\$env:PATH = \"\$env:USERPROFILE\\scoop\\shims;\$env:PATH\"
try {
$1
  if (\$LASTEXITCODE -and \$LASTEXITCODE -ne 0) { exit \$LASTEXITCODE }
} catch {
  Write-Error \$_
  exit 1
}"
    if command -v pwsh >/dev/null 2>&1; then
      pwsh -NoProfile -NonInteractive -Command "$body"
    else
      powershell.exe -NoProfile -NonInteractive -Command "$body"
    fi
  }
  bucket_repo="https://github.com/nhtera/scoop-bucket"
  bucket_name="nhtera"
  note "scoop bucket add $bucket_name $bucket_repo && scoop install $bucket_name/sonde"
  if ! pwsh_run 'Get-Command scoop -ErrorAction SilentlyContinue' | grep -q scoop; then
    note "installing scoop"
    # GitHub-hosted Windows runners run elevated; scoop's installer refuses
    # that without -RunAsAdmin.
    # shellcheck disable=SC2016 # PowerShell syntax, evaluated remotely, not by bash
    pwsh_run 'iex "& {$(irm get.scoop.sh)} -RunAsAdmin"'
  fi
  if pwsh_run "scoop bucket add $bucket_name $bucket_repo
scoop install $bucket_name/sonde"; then
    # shellcheck disable=SC2016 # PowerShell syntax, evaluated remotely, not by bash
    scoop_home="$(pwsh_run '$env:USERPROFILE' | tr -d '\r')/scoop"
    check_version_output "scoop" "$scoop_home/shims/sonde.exe"
    pwsh_run "scoop uninstall $bucket_name/sonde" >/dev/null 2>&1 || true
  else
    fail_step "scoop install $bucket_name/sonde failed"
  fi
fi

# --- 3. go install ---------------------------------------------------------
if [ -n "${SONDE_VERIFY_SKIP_GO:-}" ]; then
  skip "go install (SONDE_VERIFY_SKIP_GO set)"
elif ! command -v go >/dev/null 2>&1; then
  skip "go install (no go toolchain on this machine)"
else
  note "go install $MODULE/cmd/sonde@$VERSION"
  gobin="$(mktemp -d)"
  if GOBIN="$gobin" go install "$MODULE/cmd/sonde@$VERSION"; then
    bin="$gobin/sonde"
    [ "$PLATFORM" = windows ] && bin="$bin.exe"
    check_version_output "go install" "$bin"
  else
    fail_step "go install $MODULE/cmd/sonde@$VERSION failed"
  fi
  rm -rf "$gobin"
fi

# --- 4. Archive + checksums.txt + cosign bundle ----------------------------
if [ -n "${SONDE_VERIFY_SKIP_ARCHIVE:-}" ]; then
  skip "archive verification (SONDE_VERIFY_SKIP_ARCHIVE set)"
elif ! command -v curl >/dev/null 2>&1; then
  skip "archive verification (no curl on this machine)"
else
  ext=tar.gz
  [ "$GOOS" = windows ] && ext=zip
  asset="sonde_${PLAIN_VERSION}_${GOOS}_${GOARCH}.${ext}"
  base_url="https://github.com/$REPO/releases/download/$VERSION"
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT

  note "downloading $asset, checksums.txt, checksums.txt.sigstore.json"
  if curl -fsSL -o "$workdir/$asset" "$base_url/$asset" &&
    curl -fsSL -o "$workdir/checksums.txt" "$base_url/checksums.txt" &&
    curl -fsSL -o "$workdir/checksums.txt.sigstore.json" "$base_url/checksums.txt.sigstore.json"; then

    want="$(grep " $asset\$" "$workdir/checksums.txt" | awk '{print $1}')"
    if [ -z "$want" ]; then
      fail_step "checksums.txt has no entry for $asset"
    else
      if command -v sha256sum >/dev/null 2>&1; then
        got="$(sha256sum "$workdir/$asset" | awk '{print $1}')"
      else
        got="$(shasum -a 256 "$workdir/$asset" | awk '{print $1}')"
      fi
      if [ "$want" = "$got" ]; then
        note "checksum OK ($got)"
      else
        fail_step "checksum mismatch for $asset: want $want, got $got"
      fi
    fi

    if command -v cosign >/dev/null 2>&1; then
      # Anchored and with literal dots escaped: unescaped/unanchored, "."
      # matches any character and the pattern matches as a substring
      # anywhere in the identity, both of which would accept certificates
      # this release never produced.
      repo_escaped="${REPO//./\\.}"
      identity_re="^https://github\\.com/${repo_escaped}/\\.github/workflows/release\\.yml@refs/tags/v[0-9].*\$"
      if (cd "$workdir" && cosign verify-blob \
        --certificate-identity-regexp "$identity_re" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        --bundle checksums.txt.sigstore.json \
        checksums.txt); then
        note "cosign bundle verified"
      else
        fail_step "cosign verify-blob failed for checksums.txt"
      fi
    else
      skip "cosign bundle verification (cosign not installed)"
    fi

    archive_dir="$workdir/extracted"
    mkdir -p "$archive_dir"
    if [ "$ext" = zip ]; then
      unzip -q "$workdir/$asset" -d "$archive_dir"
      bin="$archive_dir/sonde.exe"
    else
      tar -xzf "$workdir/$asset" -C "$archive_dir"
      bin="$archive_dir/sonde"
    fi
    chmod +x "$bin" 2>/dev/null || true
    check_version_output "downloaded archive" "$bin"
  else
    fail_step "failed to download release assets from $base_url"
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo "verify-install: one or more checks FAILED" >&2
  exit 1
fi
echo "verify-install: all applicable checks passed"
