#!/usr/bin/env bash
# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Smoke checks against the deployed site: the home page and a deep docs link
# answer 200 (no redirect) with every security header, an unknown path is the
# 404 page with status 404, and hashed assets are cached as immutable.
#
#   site/deploy/smoke.sh [https://sonde.erai.dev]
set -euo pipefail

base="${1:-https://sonde.erai.dev}"
fail=0
check() { echo "$1"; [ "$2" = ok ] || fail=1; }

headers() { curl -fsS -o /dev/null -D - --retry 5 --retry-delay 5 --retry-all-errors "$1" | tr -d '\r'; }
status() { curl -sS -o /dev/null -w '%{http_code}' --retry 5 --retry-delay 5 "$1"; }

for path in / /docs/getting-started; do
  h="$(headers "$base$path")"
  code="$(printf '%s\n' "$h" | awk 'toupper($1) ~ /^HTTP/ {c=$2} END {print c}')"
  [ "$code" = 200 ] && check "$path: 200" ok || check "$path: status $code (want 200)" bad
  for name in strict-transport-security content-security-policy x-content-type-options x-frame-options referrer-policy permissions-policy; do
    if printf '%s\n' "$h" | grep -qi "^$name:"; then check "$path: $name" ok; else check "$path: missing $name" bad; fi
  done
done

code="$(status "$base/nope-$(date +%s)")"
[ "$code" = 404 ] && check "/nope: 404" ok || check "/nope: status $code (want 404)" bad
curl -sS --retry 5 "$base/nope" 2>/dev/null | grep -q "Page not found" && check "/nope: site 404 page" ok || check "/nope: not the site 404 page" bad

asset="$(curl -fsS "$base/" | grep -o '/assets/[^"]*\.js' | head -n 1)"
if [ -n "$asset" ] && headers "$base$asset" | grep -qi '^cache-control:.*immutable'; then check "$asset: immutable" ok; else check "${asset:-no asset found}: not immutable" bad; fi

exit "$fail"
