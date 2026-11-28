# Design ideas

Ideas for Wallpaper Rotator that aren't built yet, from the review in November 2026. Each has a
rough size: **S** (an afternoon), **M** (a day or two), **L** (more).

Already done from that review: the current picture's title and credit in the info box, and the
reorganised menu-bar menu (v1.6.2–v1.6.3). Since then the box also links to video.sala.company and
radio.sala.company (v1.6.6).

## Fix first

### Stop wasting downloads (M)
About a third of picture downloads are thrown away after downloading. In the log at the time of the
review: about 4,400 pictures shown and 2,331 skipped.

- **DeviantArt repeats itself.** 651 of its downloads were "same picture as one shown recently": its
  pool is too small for a 1-minute interval. Browse more pages and topics, and check the small
  preview's fingerprint (`dedupe.go`) before downloading the full picture.
- **Wikimedia Commons downloads unsuitable pictures.** 268 were the wrong size or shape. Commons
  gives width and height in its search results, so filter before downloading.

This would cut bandwidth by roughly a third and make each change faster.

### Guard the systray internals (S)
The Recent grid and the menu-bar on/off switch read private fields of fyne.io/systray (`statusItem`,
`menu`). Pin the library version and log a clear warning at startup if those fields are missing, so a
library update can't break them silently.

## Info box

- **Configurable (M):** choose the symbols (anything heat.sala.company tracks), the cities, the
  position (centre or a corner), size and opacity.
- **Smart placement (M):** put the box over the calmest part of each picture, so it never covers the
  subject. The app already analyses every picture, so a detail map is cheap.
- **More cards (S–M each):** the next calendar event, prayer times, sunrise and sunset, a heat.sala.company
  headline or a prediction-market figure.
- **sala.company shortcuts (S–M):** choose which sites get an emoji (📺 video, 📻 radio, 🔥 heat,
  🗺 maps…), and show what's on: the station playing on radio.sala.company under 📻, or the latest video
  under 📺.
- **US market holidays (S):** the list stops after 2027. Compute the rules, or read the dates from heat.
- **Calendar click (tried, removed in v1.6.5):** opening Smart Cal's drop-down from the month didn't
  work reliably. Smart Cal answers a Finder-style "reopen" by showing its panel, but centred when it
  can't see its menu-bar icon. A dedicated Smart Cal URL scheme or notification would make this
  reliable.

## Choosing pictures

- **Favourites (S–M):** a ♥ in the menu and window, and a "Favourites only" source.
- **Time-of-day mood (M):** bright pictures by day, calmer and darker ones at night (a brightness score
  per picture).
- **Learn from skips (M):** show less of what you skip or ban quickly, more of what you save.
- **Topics (M–L):** themes like space, oceans, mountains, Islamic art or calligraphy, across all sources.
- **New sources (S each):** Unsplash (free key), Rijksmuseum, NASA Astronomy Picture of the Day, Bing's
  daily image, Earth View.

## Desktop and displays

- **A different picture per display (M).**
- **Lock screen (M):** use the current picture there too, where macOS allows it.
- **Global shortcuts (S):** for example ⌃⌥→ Next, ⌃⌥← Previous, ⌃⌥⌫ Never Show Again.

## Sharing and control

- **Copy Image / Share… (S):** AirDrop, Messages, from the menu and window.
- **Shortcuts app actions (M):** Next wallpaper, Pause, Show Info Box, "pictures from this topic", so a
  Focus mode can switch to calm pictures.
- **Phone companion page (L):** a page on go.sala.company showing what's on the Mac's desktop.

## Housekeeping

- **Picture-of-the-day history (M):** a calendar of what was shown each day. History is capped at
  30 pictures today, so the cap would need raising.
- **Storage setting (S):** how many past pictures to keep, instead of the fixed 35.
- **Split `settings_darwin.m` (M):** about 900 lines of hand-built window code; one file per tab would
  make changes safer.
- **Notarisation:** needs a paid Apple Developer account; without it, new users see a Gatekeeper
  warning on first open.
