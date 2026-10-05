# 0008: Desktop updates

Status: Accepted. Date: 2026-10-05.

## Context

Sonde Desktop 0.1.0 has no update path: users download each release by hand,
and most never will. The app also promises no telemetry and that it sends
only the requests you write ([desktop.md](../desktop.md)). An update check is
a request you did not write, so it must be small, stated exactly, and easy
to turn off.

An updater is also the most powerful code path in the app: whatever it
installs runs as the user, on every machine that runs Sonde. It must not
make a compromised release feed, a compromised CI job, or a tag push enough
to reach those machines.

Wails v3 (decision 0007) ships an updater package. It handles the download,
a digest check and a helper process that swaps the app's files. It also
offers release providers, but they shape requests in ways that leak the
platform and treat a missing release as "up to date".

## Decision

### The check (D1, A1, A5)

- On by default, with a switch in Settings › History & privacy. A check is
  due once a day by the wall clock (sleep counts): it runs 5 s after the
  window opens, on an hourly tick, or when the computer wakes, whichever
  comes first once it is due. Help › Check for Updates… and the palette
  always check.
- The window app only (A6): server mode is updated with the binary that
  runs it.
- Sonde's own code lists the `desktop/v*` tags (`api.github.com`,
  `git/matching-refs`, paged) and picks the highest version on the user's
  channel. The channel is chosen by the version itself: `stable` takes
  versions without a prerelease part, `prerelease` takes all. GitHub's
  prerelease flag is not read.
- The manifest URL is built in Go from that version (`github.com`, then
  GitHub's release-asset host), as is every URL Sonde opens or downloads
  (A7). A missing manifest, or no desktop tag at
  all, is an error, never "up to date".
- The requests carry `User-Agent: Sonde-Desktop` and nothing about the user,
  the project or the machine.
- The skipped version, the channel and the last check are written by Go
  only; the page sets the switch.

### A signed manifest per release (A2, A9)

Each release carries `Sonde-Desktop-<v>.update.json`. One ed25519 signature
covers the version, a digest of the notes, and every update file's
platform, arch, name, size and SHA-512 (`internal/update/manifest`). The
app verifies it against a **pinned key set** before it downloads anything:
`k1` active, `k2` an offline standby, both pinned from 0.2.0 so a rotation
never strands an installed app. A verification failure is shown even after
a background check.

### Download through the Wails updater, and nothing else from it (A3, A4)

The Wails updater is given one provider, Sonde's: its Check hands back the
release Sonde already verified, with the signed SHA-512, and its Download
fetches the signed file from a URL built in Go, cut off past the signed
size. Wails checks the digest while it downloads. One state machine, in
the update service, owns every state the page sees; each status carries a
rising sequence number.

### Signing (A8)

The private key lives only in the `desktop-update-signing` GitHub
environment: `desktop/v*` tags only, and the maintainer approves each run.
A tag ruleset limits creating release tags to the maintainer. The signing
job runs no toolchain: it runs the manifest tool that the build job built
first and attested, after checking the attestation, and checks its output
against the committed key set.

### Install in place, per OS (D2, D3)

Only on the user's choice, and only through the close guard: unsaved tabs
are asked about first. If Sonde is still running 45 s after the hand-off,
the guard holds again and the page offers the release page.

- **macOS:** the whole bundle must belong to the user and sit on the temp
  folder's volume, and must not be translocated. The staged app's code
  signature must be Apple-anchored and carry Sonde's team ID before the
  Wails helper swaps it in.
- **Windows:** the NSIS installer runs silently in an update mode
  (`/S /UPDATE /WAITPID`), only for the machine-wide install recorded in
  HKLM. It installs into the recorded folder, waits for Sonde to quit,
  refuses to touch a `Sonde.exe` that still runs, and starts Sonde again
  through Explorer, not elevated. The installer is not code-signed (D6).
- **Linux:** the AppImage is replaced only when the app runs from a FUSE
  mount that names that very file by its path, or by its file name while a
  process of the user runs from that file (the runtime). The file must be the user's, in a folder only they can
  write. It is replaced through a folder handle and checked again through
  the descriptor that wrote it.

Anywhere else, the page says why and offers the release page.

### Versions (D4)

The CLI and the app keep separate versions and releases (decision 0007).
A desktop release's notes name the CLI engine it is built on.

## Alternatives

- **A static feed on GitHub Pages.** One more host to trust and keep in
  sync. The tag list already exists, and the signature makes its content
  irrelevant to trust.
- **The Wails release providers.** They add the platform, arch and version
  as query parameters, rewrite the arch for macOS, and read a 404 as "up to
  date". A per-artifact Wails signature could not cover the universal macOS
  build.
- **Swapping the Windows exe** (the Wails default). It does not work for
  0.1.0's machine-wide installs in Program Files. It would also skip the
  installer's registry and shortcuts.
- **Notify only, download by hand.** Simpler, but most users would stay on
  old versions. Download stays available on every error screen.
- **Signing offline only.** Safer against a CI compromise, but every
  release would wait on a manual step. The reviewer gate and the tag
  ruleset raise the bar from "push a tag" to "repository admin".

## Consequences

- Key custody is the maintainer's job: both private keys are kept in a
  password manager and on an encrypted offline drive. Rotation moves to
  `k2` and pins a new standby ([release.md](../release.md#update-signing-keys)).
  Losing both keys means every user downloads by hand once.
- Updates rely on GitHub's API and its unauthenticated rate limit. A
  background check that cannot reach GitHub is silent and retried within the
  hour; an incomplete release is checked again the next day; a failed
  verification is always shown.
- There is no automatic rollback: a release that crashes at start is fixed
  by the next one, or by downloading an older release by hand.
- On Windows, UAC shows "Unknown publisher" and SmartScreen warns on the
  first install, because the installer is unsigned. UAC is not a boundary
  against software already running as the user.
- In-place updates on Windows and Linux are tested by CI jobs (a real NSIS
  update and a real AppImage mount), not on users' machines, until a
  release reaches them (D5).
- 0.1.0 has no updater: its users download 0.2.0 by hand once.
