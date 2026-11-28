# How the November 2026 features were built

A record of one long working session with Claude (7–28 November 2026): what was asked, what was
built, and what was learned on the way. Releases v1.2.4 to v1.6.5 came out of it. (v1.5.0, Keep
Screen On, and the "keep settings safe across updates" change were made separately.)

## What was asked and built

| Version | Request | What changed |
|---|---|---|
| 1.2.4 | "Stop asking for the DeviantArt key and my password on every update." | The secret moved from the Keychain to a file only you can read (`deviantart-secret`), moved over once automatically. You enter keys once per Mac. |
| 1.3.0 | "Follow Apple's interface rules for desktop and menu-bar apps." | Four-tab window, standard app/File/Edit/Window/Help menus, ⌘, and ⌘0, Dock menu, Show in menu bar / Show in Dock, VoiceOver labels, Escape on Cancel. Notarisation was left out on purpose. |
| 1.3.1 | "Make sure the keys can't be deleted by accident." | **Remove Keys…** asks first; clicking Connect with an empty box never deletes anything. |
| 1.3.2 | "Why isn't it starting?" then "log why it quits." | Every quit is logged with its reason (menu, ⌘Q, logout, which app asked, signals, updates), and a run that ended without quitting (Force Quit, crash, power loss) is noticed at the next start. |
| 1.4.0 | "A 3×3 grid of the last 9 wallpapers in the menu, opening in the real Preview app." | **Recent Wallpapers** grid; click opens Preview with all nine (named by title), ⌥-click puts one back. |
| 1.6.0 | A mock-up of a dark widget: clock, weather, world clocks, month, markets from heat.sala.company. | The **info box**: a panel in the middle of the desktop, above the wallpaper and below windows. Prices from heat's `data.json` and `etfs.json`, weather from Open-Meteo. |
| 1.6.1 | "Give the characters a shadow" and "review it and fix bugs." | Text shadows; no downloads while auto-paused; the price time is heat's build time; weather follows a time-zone change; no "-0°". |
| 1.6.2 | "A better organised menu." | Menu grouped: now showing, changing, recent, what gets picked, on/off options, the app. New **Show Info Box** toggle. |
| 1.6.3 | Picture title in the box, a lighter shade, clickable parts. | Current picture's title and credit at the bottom; shade 50% → 22% with solid text; the time opens Clock, the weather opens Weather, heat.sala.company opens the site. |
| 1.6.3–1.6.5 | Clicking the month should open Smart Cal's drop-down. | 1.6.3 only brought Smart Cal forward (no drop-down); 1.6.4 sent a Finder-style reopen; still unreliable, so 1.6.5 removed it. |

## Decisions

- **The info box is a live window, not drawn onto the picture.** A clock baked into a wallpaper is
  wrong a minute later; a window at desktop level redraws each minute and works over every picture.
- **Clicks pass through the box.** Only small invisible windows over the links catch the mouse, so you
  can still click and drag on the desktop.
- **Text solid, background light.** The shade is 22% so the picture shows; the text is fully opaque
  with a dark shadow so it reads on any picture.
- **Off by default for new users,** on for the owner; a checkbox in Settings → General and a menu toggle.
- **Work goes straight to `main`,** and a release is a tag, published only after an explicit OK.

## Lessons

- **fyne.io/systray quits with `[NSApp stop]`, not terminate,** so "will terminate" never fires for the
  app's own Quit. Quit logging had to run after `systray.Run` returns, plus a handler for the quit
  Apple Event to learn who asked.
- **The Keychain asks again whenever the app's signature changes,** which is every update for an app
  that isn't notarised. A 0600 file in Application Support avoids it.
- **JSON `null` crashes Objective-C drawing.** A Go nil slice became `NSNull` and `-count` threw. Fixed
  on both sides: Go sends empty rows, and the drawing code checks types.
- **A window at desktop level sits under Finder's desktop-icon window,** so it can never get clicks.
  Click areas need their own windows just above the icon level, with a barely visible fill (fully
  clear pixels let clicks through).
- **Activating a menu-bar app shows nothing.** Smart Cal needed a Finder-style "reopen" to show its
  panel, and even then it centres the panel when its menu-bar icon is hidden (e.g. behind the notch).
- **Checking a desktop window without seeing the screen:** list windows with
  `CGWindowListCopyWindowInfo`, capture one with `screencapture -l <id>`, and lay that capture over
  the current wallpaper file to judge transparency.
- **Release check:** after each tag, download the published zip, read its version with PlistBuddy and
  run `codesign --verify --deep`.
