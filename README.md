# Wallpaper Rotator for Mac

A tiny macOS menu-bar app, written in Go, that changes your desktop wallpaper every few
minutes with **public-domain** pictures: nebulae and Earth from orbit, landscape paintings
from museum collections, and Wikimedia Commons' best photos. It's family-friendly by design
and needs no accounts or API keys.

## Features
- 🖼 Menu-bar icon only, with no Dock icon.
- ⚙️ A **Settings window** that opens on first launch, whenever you open the app again
  (Finder, Launchpad, Spotlight), or from the menu's **Settings…**.
- ⏱ Changes every 4 minutes by default (1, 4, 10, 30 or 60 minutes available).
- 🌍 Four public-domain / CC0 sources that you can switch on and off individually.
- 🖥 Covers every display. When you switch to another desktop (Space), the latest picture
  is applied there too.
- 🔁 Doesn't repeat pictures (it remembers the last 2,000).
- 💾 Saves pictures you like to `~/Pictures/Wallpapers`, and links to each picture's
  source page for credit and license.

## Sources
| Source | What you get | License |
|---|---|---|
| [NASA Image Library](https://images.nasa.gov) | Nebulae, galaxies, launches, auroras, Earth from orbit | Public domain |
| [Wikimedia Commons Featured Pictures](https://commons.wikimedia.org/wiki/Commons:Featured_pictures) | Award-winning photos of nature, cities, animals | Only Public domain / CC0 files are used |
| [Cleveland Museum of Art Open Access](https://openaccess-api.clevelandart.org) | Landscape and seascape paintings, prints, early photography | CC0 |
| [Smithsonian Open Access](https://www.si.edu/openaccess) (American Art Museum) | Hudson River School, the American West | CC0 |

Search results are cached and used up one at a time, so each API is queried rarely. A source
that fails is skipped for 15 minutes, or for 2 hours if it reports a rate limit.

The Smithsonian uses the shared `DEMO_KEY` (about 50 requests a day). For more, get a free key at
https://api.data.gov/signup and add it to the config file:
```json
"smithsonian_api_key": "YOUR_KEY"
```

## Keeping it safe
1. **Safe sources.** NASA, *featured* Commons photos, and museum searches limited to scenery
   (sea, mountains, sunsets…).
2. **Blocklist.** Titles, descriptions, keywords and categories are checked against a list of
   nudity, gore and medical terms ([`filter.go`](filter.go)). Art sources also get an extra list
   (Venus, nymphs, bathers…).
3. **Shape check.** Only landscape images at least 1920px wide are used.

No filter is perfect. If something slips through, add the word to `filter.go`, or keep only
NASA switched on.

## Install
Requires macOS 12+, [Go](https://go.dev/dl/) 1.22+ and the Xcode Command Line Tools
(`xcode-select --install`).

```bash
git clone https://github.com/jamalxcode/WallpaperRotatorMac.git
cd WallpaperRotatorMac
make install    # builds WallpaperChanger.app, copies it to /Applications, launches it
```

To start it automatically, use the menu's **Start at Login**, or go to System Settings →
General → Login Items → **+** → WallpaperChanger.

> **Can't see the icon?** On MacBooks with a notch, macOS hides menu-bar icons that don't
> fit. Just open the app again to get the Settings window, or remove a few other icons
> (⌘-drag them off) to make room.

### Other commands
```bash
make run                                       # build and run from the project folder
make once                                      # change the wallpaper once, no menu bar
go run . --once --dry --source Cleveland       # test one source without changing the wallpaper
```

## Settings
Open the app again (from Finder, Launchpad or Spotlight) to bring up the Settings window. It's
handy when the menu-bar icon is hidden. It shows the current picture and its credit, and has:
**Next Wallpaper**, **Open Image Page**, **Save to Pictures**, the change interval, the picture
sources, **Pause** and **Start at login**.

The menu-bar icon has the same options, plus **Settings…** to open the window. Changes made in
one place show up in the other.

## Files
| What | Where |
|---|---|
| Settings | `~/Library/Application Support/WallpaperChanger/config.json` |
| Image cache (last 8) | `~/Library/Application Support/WallpaperChanger/images/` |
| Log | `~/Library/Logs/WallpaperChanger.log` |
| Login item | `~/Library/LaunchAgents/com.yoda.wallpaperchanger.plist` |

## How it works
| File | Role |
|---|---|
| `main.go` | Menu bar ([fyne.io/systray](https://github.com/fyne-io/systray)), timer, actions shared by menu and window |
| `settings_darwin.m/.h/.go` | Native Cocoa Settings window and the "app opened again" handling |
| `sources.go`, `sources_museums.go` | API clients for each source |
| `fetch.go` | Per-source candidate cache, back-off, download and size checks |
| `filter.go` | Content blocklists |
| `wallpaper_darwin.go` | Sets the wallpaper through `NSWorkspace` (cgo / Objective-C) and re-applies it on Space switches |
| `store.go`, `login.go`, `icon.go` | Settings, history, login item, generated menu-bar icon |

## Known limitations
- macOS only lets apps set the wallpaper of the **current** Space. Other Spaces get the new
  picture when you switch to them.
- Full-screen app Spaces have no wallpaper, so nothing changes there.

## License
[MIT](LICENSE). The pictures themselves belong to their sources (public domain / CC0). Use
**Open Image Page** for each picture's credit.
