# Native checklist

What the browser tests cannot drive: native dialogs, menus, the trash, the
clipboard, quarantine, Gatekeeper, the system theme and the launch
environment. Run it on macOS against the **notarized disk image of the
release candidate** (downloaded through a browser, so it is quarantined),
before tagging. Run the Windows and Linux parts on those systems when the
release changes anything they touch.

Use a copy of `testdata/shop-api` as the project, with the fixture API
running (`desktop/bin/fixture-server`, built by `make desktop-e2e`).
Tick each item, and write down the OS version and the build (version, commit).

## 1. Gatekeeper and first launch (macOS)

- [ ] Download the `.dmg` in a browser. Open it, copy the app to Applications,
      eject the image, open the app. **Expect:** no "unidentified developer"
      or "damaged" dialog (at most the one-time "downloaded from the
      Internet" confirmation).
- [x] `spctl -a -vv -t install Sonde-Desktop-X.Y.Z-macos-universal.dmg`
      says `accepted` and `source=Notarized Developer ID`. *(macOS 15.7.3, 0.1.0-rc.1)*
- [ ] `xcrun stapler validate` on the `.dmg` succeeds. With networking off,
      the app still opens.
- [x] `codesign --verify --deep --strict -vv /Applications/Sonde.app`
      passes, and `codesign -dv /Applications/Sonde.app` shows
      `flags=0x10000(runtime)` (the hardened runtime). *(macOS 15.7.3, 0.1.0-rc.1)*
- [x] `file .../Contents/MacOS/sonde-desktop` lists `x86_64` and `arm64`. *(macOS 15.7.3, 0.1.0-rc.1)*
- [ ] The Windows build only: SmartScreen shows "Windows protected your PC";
      More info › Run anyway installs and starts the app, and the installer's
      SHA-256 matches `checksums.txt`.

## 2. Native dialogs and handles

- [x] **Open a folder** (⌘O, and the welcome screen's button) shows the
      native picker. Cancel: nothing changes, no error toast. Choose
      `shop-api`: the tree fills; the folder appears in the recent list
      after a restart. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] **Run with a data file…** picks a `.csv` or `.json`: the run executes
      once per row. Cancel the dialog: nothing runs. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Settings › Certificates: pick a CA bundle. The page shows the file's
      name, not its path. Remove it. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Import: pick a Postman collection and a folder through the dialogs;
      the preview lists the files. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Import a Postman collection whose requests use a variable it never
      defines (a cookie from a Postman environment) into an empty folder.
      **Expect:** the collection's variables show in Environments at once;
      the result lists the variable to define; **Define…** saves it as a
      secret (`secrets/<env>.secrets`, mode 0600) and the row goes. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*
- [x] Run a file with an undefined `{{name}}`: **Define name…** on the
      error, and **Define…** on the editor's hover (the card shows whole
      on line 1 too), open the same dialog; after Save, **Run again**
      sends the request. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*
- [x] Form › Body › file: pick a file outside the project. The app offers
      to copy it into `assets/`; it never overwrites an existing file. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] A selection is single use: pick a data file, wait over 5 minutes, then
      run. **Expect:** "the selection expired; choose the file again", not
      a run. *(macOS 15.7.3, local build of `93c8a03`)* (tried with a body file's copy)
- [x] Save response (⌥⌘S) shows the native save panel with a suggested name,
      and the saved file holds the body exactly as the server sent it. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] The Test run panel's export: choose a folder; a new `sonde-<format>-<time>-…` folder appears in it. *(macOS 15.7.3, local build of `93c8a03`)*

## 3. Menus and keys

- [x] The menu bar has the app, File, Edit, View, Window and Help menus,
      **no Reload and no Force Reload**. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Help › Sonde Desktop Help opens the desktop guide in the default
      browser; the app's window keeps its tabs and unsaved edits. *(macOS 15.7.3, local build of `fix/desktop-help-menu`)*
- [x] Edit › Copy, Paste, Select All, Undo and Redo work in the editor and in
      the form's fields. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] View: Zoom In, Zoom Out and Actual Size work; Full Screen toggles. *(macOS 15.7.3, local build of `93c8a03`)* (Zoom Out stops at actual size)
- [x] ⌘R runs the file and does not reload the page. ⌘S saves, ⌘K opens the
      palette, ⌘/ toggles a comment in the editor, ⌘G asks for a line, `?`
      opens the shortcuts sheet. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Closing the window or quitting with an unsaved tab asks first. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] ⌘W closes the active tab (asking first when it is unsaved) and leaves the window open; ⇧⌘W closes the window. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Right-clicking a tab shows its menu; Close other tabs with an unsaved tab among them asks once, and Cancel keeps every tab. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] In the window, ⌃Tab / ⌃⇧Tab switch tabs (also from the editor), ⇧⌘T reopens the last closed tab, and a pinned tab survives Close all tabs. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*

## 4. Trash and Reveal

- [x] File tree › **Move to Trash** on a file and on a folder: the item is in
      the Finder's Trash (and can be restored); the tree updates. Try names
      with spaces, quotes, `$` and non-ASCII characters. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] **Reveal in Finder** selects the file. On Windows, Explorer opens with
      the file selected; on Linux, the file manager opens its folder. *(macOS 15.7.3, local build of `93c8a03`)* (macOS only)
- [x] A secrets file (`*.secrets`) offers no Rename or Duplicate, and cannot
      be opened from the tree. *(macOS 15.7.3, local build of `93c8a03`)*

## 5. The concealed clipboard

- [ ] With a clipboard manager running (one that records history), run a
      request that uses a secret, then **Copy as › curl**, holding ⌥ while
      choosing the item. **Expect:** a confirmation naming the clipboard.
      After confirming, paste into a text editor: the real token is there.
      **The clipboard manager did not record it.**
- [x] Without ⌥, the copied command shows `***` or variable references, no
      real value. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Wait 60 seconds, paste again: the clipboard is empty. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Reveal again, then copy other text within 60 seconds: after the
      minute, **the other text is still on the clipboard** (it was not
      cleared). *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Copy as › sonde with ⌥: the command holds real values, and again is
      cleared after 60 s. *(macOS 15.7.3, local build of `93c8a03`)*
- [ ] Windows: the revealed text does not appear in the clipboard history
      (Win+V).

## 6. Open externally and quarantine

- [x] Run a request that returns a PDF, then **Open in default app**. The PDF
      opens in Preview; macOS treats it as downloaded (`xattr -l` on the
      file, whose folder is the app's private temp folder, shows
      `com.apple.quarantine`). A PNG opens in Preview, a JSON body in the
      default text editor. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] An HTML body opens as `.txt`, never in a browser. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] A body with a secret shows `***` in the opened file (the redacted body
      is what is written), while **Save response** writes the real bytes. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Quit the app: the temp files are gone. *(macOS 15.7.3, local build of `93c8a03`)*
- [ ] Windows: the file's Properties show the "This file came from another
      computer" Unblock option.

## 7. Appearance

- [x] Settings › General › Theme at Sync with system, Day theme Solarized
      Light and Night theme Dracula. System Settings › Appearance: switch
      Light and Dark while the app runs. The app follows at once (editor,
      results, dialogs, the search panel ⌘F included) and the Active badge
      moves to the slot in use. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Launch with Manual, switch to Sync in Settings, then flip the system
      appearance: the app follows (the window's look is never pinned). *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Manual with Monokai: flipping the system changes nothing in the page;
      native menus follow the system. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] The rail's theme button and "Toggle day / night theme" switch between
      the Day and Night themes (from Sync: to Manual with the other slot's). *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Select theme… (⌘K): arrows preview each theme, Esc puts the theme
      back, Enter keeps it; with Sync on a dark system it becomes the Night
      theme. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Quit and launch with Manual Dracula on a light system, then with Sync
      and Night Dracula on a dark system: no frame in another theme. Note
      any plain white frame before the page paints (a known webview limit). *(macOS 15.7.3, local build of `93c8a03`)* One plain white frame with Manual Dracula on a light system; none on a dark one.
- [x] `task dev`: the app loads from Vite with hot reload, in the theme set. *(macOS 15.7.3, local build of `a0e3cf7`)*
- [x] Resize to the minimum (900 × 560) and below 1024 px wide: the side panel
      becomes an overlay and the results stay docked. *(macOS 15.7.3, local build of `93c8a03`)*

## 8. The launch environment

- [x] Put `export HURL_VARIABLE_origin=terminal` in `~/.zshrc` (or set it in
      the terminal), and a `{{origin}}` in a request. *(macOS 15.7.3, local build of `93c8a03`)* Set in the terminal.
- [x] Open the app **from the terminal** by running
      `.../Contents/MacOS/sonde-desktop` directly (it inherits the shell's
      environment): the overrides chip shows the variable and the request sends `terminal`. *(macOS 15.7.3, local build of `93c8a03`)*
- [x] Open the app **from the Finder, the Dock and Spotlight**: the chip does
      not show it and `{{origin}}` is undefined. The request fails with the
      undefined-variable error, as expected. *(macOS 15.7.3, local build of `93c8a03`)* The Finder, and `open` with a clean environment (the Dock and Spotlight launch the same way).
- [ ] A config file at `$HOME/.config/hurl/config` (the path used when
      `XDG_CONFIG_HOME` is not set, as in a Finder launch) with
      `--header "X-From: config"` and `--insecure` applies from every launch
      method, and the chip shows both. (Checked at `93c8a03` with the old
      `$HOME/config/hurl/config` path and `--header` only; re-check after
      the full config file.)

## 9. Responses in the native webview

- [x] Run `json.hurl` from the perf project (a ~50 MB JSON response): the
      body arrives, the tree appears, the window stays responsive, and
      cancelling the request mid-body stops the download. *(macOS 15.7.3, local build of `a0e3cf7`)* 48 MB: a body over 50 MB shows no tree. Cancelled with Stop (⌘.) on a throttled server.
- [x] An HTML response previews in the sandboxed frame; a script in it does
      not run. A PDF previews, or offers Open in default app. *(macOS 15.7.3, local build of `a0e3cf7`)*
- [x] A 5 MB JSON body shows the tree and Raw views, and the panel's search
      finds a value in it. *(macOS 15.7.3, local build of `a0e3cf7`)*

## 10. Linux (AppImage)

- [ ] On Ubuntu 24.04, Debian 13 and Fedora 40, `chmod +x` and run the
      AppImage: the window opens, and a request runs.
- [ ] The JSON tree for a large body (WebKitGTK is slower than the macOS
      webview) is usable; note the time for the 50 MB file.
- [ ] Open a folder, Reveal, Trash and Open in default app use the desktop's
      own tools.

## 11. Performance table

On an awake, unlocked display (a locked or sleeping screen draws no frames
and fails the tour), with the packaged app:

```sh
cd desktop
../bin/task darwin:package          # or the packaging task of this OS
go build -o bin/fixture-server ./cmd/fixture-server
node scripts/perf.mjs               # --harness adds the browser tests' rows
```

It starts the app with `--perf-trace` on a generated 1000-file project, with
the fixture API serving a ~50 MB response, then prints the budget table
(native rows measured in the shipped app, harness rows, the initial JS and
the download size). It exits 1 if a measured budget is missed.

- [x] Every budget is met, or each miss is explained and accepted.
      Keystroke to paint: 18.0 ms p95 against 16 ms, accepted for 0.1.0. The
      tour times to the next frame, 16.7 ms at 60 Hz, so 18 ms is the first
      frame.
- [x] **Paste the table into the release pull request**, with the machine
      (for example "MacBook Air M1, macOS 15.x"). MacBook Pro M4 Pro, macOS
      15.7.3, in #3.

## 12. Updates

Against a release candidate's update to the next one
([release.md](../docs/release.md#rehearsing-an-updater-change)). Windows and
Linux stay unticked, marked "CI only (D5)" with a link to the green
`desktop-install-windows` and `desktop-install-linux` runs, until a machine is
available.

- [ ] With the switch on and no check in the last 24 hours, the update is
      offered within 5 seconds of opening the window. *(macOS / Windows /
      Linux)*
- [ ] After a sleep longer than 24 hours, it is offered within an hour of
      waking.
- [ ] Offline: no message from the daily check; Help › Check for Updates…
      says "Could not reach GitHub." with Download.
- [ ] Install shows progress; with an unsaved tab, Restart to update asks
      first ("Restart without saving"), and Cancel keeps everything.
- [ ] The new version runs. **macOS:** no Gatekeeper dialog. **Windows:** not
      elevated, in the same folder, with the new version in Installed apps.
      **Linux:** the AppImage file is the new one.
- [ ] Skip this version: the next daily check stays quiet; a manual check
      shows "(skipped)" with Install anyway; Settings › Show skipped updates
      again clears it.
- [ ] Two Sonde windows open: Restart waits with "Quit the other Sonde
      windows first", and works after the other one quits (Check again).
- [ ] The fallbacks show their reason and Download: the app run from the
      disk image (translocated); a bundle with a file owned by root (nothing
      is deleted); an AppImage in a group-writable folder; the administrator
      prompt declined.
- [ ] Switch off: no request of the app's own in 2 minutes (a proxy log).
- [ ] With Settings › Network › Proxy set, the check goes through that proxy
      (the proxy log shows `api.github.com`).
- [ ] The negative checks of the rehearsal (signature removed, version,
      notes, a flipped byte, oversize, missing manifest) are each refused.
