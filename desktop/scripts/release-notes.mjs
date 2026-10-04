// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Prints a desktop release's notes. They are the GitHub release's text and,
// signed in the update manifest, what the app shows when it offers the
// update. Run from desktop/:
//   node scripts/release-notes.mjs VERSION ENGINE REPO

import { pathToFileURL } from "node:url";

export function releaseNotes(v, engine, repo) {
  const lines = [
    `Sonde Desktop ${v}: a desktop app for .hurl files, built on the sonde CLI.`,
    "",
    `Engine: Sonde CLI ${engine.replace("+", " + ")}`,
    "",
  ];
  // 0.1.0 has no updater; 0.2.0 (and its candidates) is the one download by hand.
  if (/^0\.2\.0(-|$)/.test(v)) {
    lines.push("0.1.0 has no updater: download this release once; later releases install from the app.", "");
  }
  lines.push(
    "- macOS (universal): signed and notarized disk image.",
    '- Windows (x64, ARM64): the installers are not signed yet; Windows SmartScreen shows "Windows protected your PC" on first launch: choose More info, then Run anyway. Verify the download against checksums.txt first.',
    "- Linux (x86_64): AppImage (GTK 4, WebKitGTK 6.0: Ubuntu 24.04+, Debian 13, Fedora 40+).",
    "- Server mode (Sonde-Desktop-Server-*): the app in a browser, for a remote dev box: loopback only, with a one-time launch link and a token.",
    "- On Windows and Linux, installing updates in place is tested by automated tests only.",
    "",
    "No account and no telemetry. Run history stays on this computer, with secrets, credential headers and cookie values masked. Server mode listens on loopback only and needs its token.",
    "",
    `Verify: checksums.txt is signed with cosign (keyless, this workflow's identity); every file has a build provenance attestation (gh attestation verify FILE --repo ${repo}). Sonde-Desktop-${v}.update.json lists the update files with their SHA-512, signed with the app's pinned update key.`,
  );
  return lines.join("\n") + "\n";
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [v, engine, repo] = process.argv.slice(2);
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(v ?? "") || !engine || !repo) {
    console.error("usage: node scripts/release-notes.mjs VERSION ENGINE REPO");
    process.exit(2);
  }
  process.stdout.write(releaseNotes(v, engine, repo));
}
