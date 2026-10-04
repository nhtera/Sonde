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
- [ ] `spctl -a -vv -t install Sonde-Desktop-X.Y.Z-macos-universal.dmg`
      says `accepted` and `source=Notarized Developer ID`.
- [ ] `xcrun stapler validate` on the `.dmg` succeeds. With networking off,
      the app still opens.
- [ ] `codesign --verify --deep --strict -vv /Applications/Sonde.app`
      passes, and `codesign -dv /Applications/Sonde.app` shows
      `flags=0x10000(runtime)` (the hardened runtime).
- [ ] `file .../Contents/MacOS/sonde-desktop` lists `x86_64` and `arm64`.
- [ ] The Windows build only: SmartScreen shows "Windows protected your PC";
      More info › Run anyway installs and starts the app, and the installer's
      SHA-256 matches `checksums.txt`.

## 2. Native dialogs and handles

- [ ] **Open a folder** (⌘O, and the welcome screen's button) shows the
      native picker. Cancel: nothing changes, no error toast. Choose
      `shop-api`: the tree fills; the folder appears in the recent list
      after a restart.
- [ ] **Run with a data file…** picks a `.csv` or `.json`: the run executes
      once per row. Cancel the dialog: nothing runs.
- [ ] Settings › Certificates: pick a CA bundle. The page shows the file's
      name, not its path. Remove it.
- [ ] Import: pick a Postman collection and a folder through the dialogs;
      the preview lists the files.
- [x] Import a Postman collection whose requests use a variable it never
      defines (a cookie from a Postman environment) into an empty folder.
      **Expect:** the collection's variables show in Environments at once;
      the result lists the variable to define; **Define…** saves it as a
      secret (`secrets/<env>.secrets`, mode 0600) and the row goes. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*
- [x] Run a file with an undefined `{{name}}`: **Define name…** on the
      error, and **Define…** on the editor's hover (the card shows whole
      on line 1 too), open the same dialog; after Save, **Run again**
      sends the request. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*
- [ ] Form › Body › file: pick a file outside the project. The app offers
      to copy it into `assets/`; it never overwrites an existing file.
- [ ] A selection is single use: pick a data file, wait over 5 minutes, then
      run. **Expect:** "the selection expired; choose the file again", not
      a run.
- [ ] Save response (⌥⌘S) shows the native save panel with a suggested name,
      and the saved file holds the body exactly as the server sent it.
- [ ] The Test run panel's export: choose a folder; a new `sonde-<format>-<time>-…` folder appears in it.

## 3. Menus and keys

- [ ] The menu bar has the app, File, Edit, View, Window and Help menus,
      **no Reload and no Force Reload**.
- [ ] Edit › Copy, Paste, Select All, Undo and Redo work in the editor and in
      the form's fields.
- [ ] View: Zoom In, Zoom Out and Actual Size work; Full Screen toggles.
- [ ] ⌘R runs the file and does not reload the page. ⌘S saves, ⌘K opens the
      palette, ⌘/ toggles a comment in the editor, ⌘G asks for a line, `?`
      opens the shortcuts sheet.
- [ ] Closing the window or quitting with an unsaved tab asks first.
- [ ] ⌘W closes the active tab (asking first when it is unsaved) and leaves the window open; ⇧⌘W closes the window.
- [ ] Right-clicking a tab shows its menu; Close other tabs with an unsaved tab among them asks once, and Cancel keeps every tab.
- [x] In the window, ⌃Tab / ⌃⇧Tab switch tabs (also from the editor), ⇧⌘T reopens the last closed tab, and a pinned tab survives Close all tabs. *(macOS 15.7.3, local ad-hoc build, not the notarized RC)*

## 4. Trash and Reveal

- [ ] File tree › **Move to Trash** on a file and on a folder: the item is in
      the Finder's Trash (and can be restored); the tree updates. Try names
      with spaces, quotes, `$` and non-ASCII characters.
- [ ] **Reveal in Finder** selects the file. On Windows, Explorer opens with
      the file selected; on Linux, the file manager opens its folder.
- [ ] A secrets file (`*.secrets`) offers no Rename or Duplicate, and cannot
      be opened from the tree.

## 5. The concealed clipboard

- [ ] With a clipboard manager running (one that records history), run a
      request that uses a secret, then **Copy as › curl**, holding ⌥ while
      choosing the item. **Expect:** a confirmation naming the clipboard.
      After confirming, paste into a text editor: the real token is there.
      **The clipboard manager did not record it.**
- [ ] Without ⌥, the copied command shows `***` or variable references, no
      real value.
- [ ] Wait 60 seconds, paste again: the clipboard is empty.
- [ ] Reveal again, then copy other text within 60 seconds: after the
      minute, **the other text is still on the clipboard** (it was not
      cleared).
- [ ] Copy as › sonde with ⌥: the command holds real values, and again is
      cleared after 60 s.
- [ ] Windows: the revealed text does not appear in the clipboard history
      (Win+V).

## 6. Open externally and quarantine

- [ ] Run a request that returns a PDF, then **Open in default app**. The PDF
      opens in Preview; macOS treats it as downloaded (`xattr -l` on the
      file, whose folder is the app's private temp folder, shows
      `com.apple.quarantine`). A PNG opens in Preview, a JSON body in the
      default text editor.
- [ ] An HTML body opens as `.txt`, never in a browser.
- [ ] A body with a secret shows `***` in the opened file (the redacted body
      is what is written), while **Save response** writes the real bytes.
- [ ] Quit the app: the temp files are gone.
- [ ] Windows: the file's Properties show the "This file came from another
      computer" Unblock option.

## 7. Appearance

- [ ] Settings › General › Theme at Sync with system, Day theme Solarized
      Light and Night theme Dracula. System Settings › Appearance: switch
      Light and Dark while the app runs. The app follows at once (editor,
      results, dialogs, the search panel ⌘F included) and the Active badge
      moves to the slot in use.
- [ ] Launch with Manual, switch to Sync in Settings, then flip the system
      appearance: the app follows (the window's look is never pinned).
- [ ] Manual with Monokai: flipping the system changes nothing in the page;
      native menus follow the system.
- [ ] The rail's theme button and "Toggle day / night theme" switch between
      the Day and Night themes (from Sync: to Manual with the other slot's).
- [ ] Select theme… (⌘K): arrows preview each theme, Esc puts the theme
      back, Enter keeps it; with Sync on a dark system it becomes the Night
      theme.
- [ ] Quit and launch with Manual Dracula on a light system, then with Sync
      and Night Dracula on a dark system: no frame in another theme. Note
      any plain white frame before the page paints (a known webview limit).
- [ ] `task dev`: the app loads from Vite with hot reload, in the theme set.
- [ ] Resize to the minimum (900 × 560) and below 1024 px wide: the side panel
      becomes an overlay and the results stay docked.

## 8. The launch environment

- [ ] Put `export HURL_VARIABLE_origin=terminal` in `~/.zshrc` (or set it in
      the terminal), and a `{{origin}}` in a request.
- [ ] Open the app **from the terminal** by running
      `.../Contents/MacOS/sonde-desktop` directly (it inherits the shell's
      environment): the overrides chip shows the variable and the request sends `terminal`.
- [ ] Open the app **from the Finder, the Dock and Spotlight**: the chip does
      not show it and `{{origin}}` is undefined. The request fails with the
      undefined-variable error, as expected.
- [ ] A config file at `$HOME/config/hurl/config` (the path used when
      `XDG_CONFIG_HOME` is not set, as in a Finder launch) with
      `--header "X-From: config"` applies from every launch method, and the
      chip shows it.

## 9. Responses in the native webview

- [ ] Run `json.hurl` from the perf project (a ~50 MB JSON response): the
      body arrives, the tree appears, the window stays responsive, and
      cancelling the request mid-body stops the download.
- [ ] An HTML response previews in the sandboxed frame; a script in it does
      not run. A PDF previews, or offers Open in default app.
- [ ] A 5 MB JSON body shows the tree and Raw views, and the panel's search
      finds a value in it.

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

- [ ] Every budget is met, or each miss is explained and accepted.
- [ ] **Paste the table into the release pull request**, with the machine
      (for example "MacBook Air M1, macOS 15.x").
