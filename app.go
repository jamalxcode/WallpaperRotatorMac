package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var intervals = []int{1, 4, 10, 30, 60}

var sourceLabels = map[string]string{
	"NASA":        "NASA (space & Earth)",
	"Wildlife":    "Wildlife & Nature (US parks, NOAA)",
	"Wikimedia":   "Wikimedia Commons (featured)",
	"Cleveland":   "Cleveland Museum of Art",
	"Smithsonian": "Smithsonian American Art",
	"DeviantArt":  "DeviantArt (needs your keys)",
}

const daKeychainAccount = "deviantart-client-secret"

// App owns the settings and state; the menu and Settings window both call its
// exported methods.
type App struct {
	mu      sync.Mutex
	cfg     Config
	sources []Source
	fetcher *Fetcher
	history *History
	current Shown
	sys     SystemState
	missed  bool // a change was skipped while auto-paused
	offline bool // showing saved pictures because there's no internet

	daSecret       string
	daSecretLoaded bool

	busy  atomic.Bool
	reset chan struct{}
	menu  *Menu
}

func NewApp() *App {
	a := &App{
		cfg:     loadConfig(),
		history: loadHistory(historyPth),
		reset:   make(chan struct{}, 1),
		sys:     SystemState{Online: true},
	}
	setMainDisplayOnly(a.cfg.MainDisplayOnly)
	a.fetcher = NewFetcher(loadIDSet(seenPath, 2000), loadIDSet(bannedPath, 0), loadHashSet(looksPath, 2000))
	a.seedFingerprints()
	a.fetcher.check = func(path string, img Image) error {
		r, ok := visionCheck(path)
		if !ok {
			return nil // Vision unavailable: rely on the other filters
		}
		return visionVerdict(img.Kind, r)
	}
	a.sources = []Source{
		NASA{}, Wildlife{}, Wikimedia{}, ClevelandArt{}, Smithsonian{Key: a.cfg.SmithsonianKey},
		DeviantArt{creds: a.deviantCreds},
	}
	if items := a.history.All(); len(items) > 0 {
		a.current = items[len(items)-1]
	}
	// Move a DeviantArt secret from the old config field into its own file.
	if s := a.cfg.LegacySecret; s != "" {
		if err := saveDeviantSecret(s); err == nil {
			a.cfg.LegacySecret = ""
			a.daSecret, a.daSecretLoaded = s, true
		} else {
			log.Print("saving DeviantArt secret: ", err)
		}
	}
	a.cfg.save()
	return a
}

// seedFingerprints fingerprints pictures shown before this feature existed.
func (a *App) seedFingerprints() {
	if _, err := os.Stat(looksPath); err == nil {
		return
	}
	for _, s := range a.history.All() {
		if h, err := fileHash(s.Path); err == nil {
			a.fetcher.looks.Add(h)
		}
	}
}

func (a *App) deviantCreds() (id, secret string, skipAI bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.daSecretLoaded && a.cfg.DeviantArtClientID != "" {
		a.daSecret, a.daSecretLoaded = loadDeviantSecret(), true
	}
	return a.cfg.DeviantArtClientID, a.daSecret, !a.cfg.DeviantArtAllowAI
}

func (a *App) weights() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	w := make(map[string]int, len(a.cfg.Weights))
	for k, v := range a.cfg.Weights {
		w[k] = v
	}
	return w
}

// autoPauseReason says why changes are on hold right now ("" = not paused).
func (a *App) autoPauseReason() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case a.cfg.Paused:
		return "Paused"
	case a.sys.Locked:
		return "Paused while the screen is locked"
	case a.sys.ScreensAsleep:
		return "Paused while the display is asleep"
	case a.sys.LowPower && !a.cfg.KeepRunning:
		return "Paused: Low Power Mode is on"
	case a.sys.Expensive && !a.cfg.KeepRunning:
		return "Paused: on a phone hotspot or Low Data Mode"
	}
	return ""
}

// ---- main loop

func (a *App) Run() {
	startSystemMonitor(a.onSystemState)
	if a.autoPauseReason() == "" {
		a.change()
	} else {
		a.mu.Lock()
		a.missed = !a.cfg.Paused
		a.mu.Unlock()
		a.refreshStatus(false)
	}
	for {
		a.mu.Lock()
		d := time.Duration(a.cfg.IntervalMin) * time.Minute
		a.mu.Unlock()
		t := time.NewTimer(d)
		select {
		case <-t.C:
			if reason := a.autoPauseReason(); reason != "" {
				a.mu.Lock()
				a.missed = reason != "Paused"
				a.mu.Unlock()
				a.syncUI()
				continue
			}
			a.change()
		case <-a.reset:
			t.Stop()
		}
	}
}

func (a *App) kick() {
	select {
	case a.reset <- struct{}{}:
	default:
	}
}

func (a *App) onSystemState(st SystemState) {
	a.mu.Lock()
	prev := a.sys
	a.sys = st
	catchUp := a.missed
	a.mu.Unlock()
	if prev != st {
		log.Printf("system: locked=%v asleep=%v lowPower=%v expensive=%v online=%v",
			st.Locked, st.ScreensAsleep, st.LowPower, st.Expensive, st.Online)
	}
	a.syncUI()
	// Coming back (unlock, wake, power restored, back online): catch up once.
	if catchUp && a.autoPauseReason() == "" {
		a.mu.Lock()
		a.missed = false
		a.mu.Unlock()
		go a.change()
		a.kick()
	} else if prev.Online != st.Online && st.Online {
		a.mu.Lock()
		wasOffline := a.offline
		a.mu.Unlock()
		if wasOffline && a.autoPauseReason() == "" {
			go a.change()
			a.kick()
		}
	}
}

// change shows a new wallpaper: fetched, or a saved one when offline.
func (a *App) change() {
	if !a.busy.CompareAndSwap(false, true) {
		return
	}
	defer a.busy.Store(false)
	a.refreshStatusText("Finding a new picture…", "", true)

	a.mu.Lock()
	online := a.sys.Online
	a.mu.Unlock()

	var shown Shown
	var err error
	if online {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		var img Image
		var path string
		img, path, err = a.fetcher.Fetch(ctx, a.sources, a.weights())
		cancel()
		if err == nil {
			shown = Shown{ID: img.ID, Title: img.Title, Credit: img.Credit, PageURL: img.PageURL, Source: img.Source, Path: path}
		}
	} else {
		err = errors.New("offline")
	}

	offline := false
	if err != nil && (!online || isNetworkError(err)) {
		// No internet: rotate through pictures already on disk.
		var ok bool
		shown, ok = offlinePick(a.history.All(), savedDir, a.cur().Path, rand.New(rand.NewPCG(rand.Uint64(), 1)))
		if ok {
			err, offline = nil, true
		}
	}
	if err == nil {
		err = setWallpaper(shown.Path)
	}
	if err != nil {
		log.Print("change failed: ", err)
		a.refreshStatus(false)
		if a.cur().ID == "" {
			msg, sub := "Couldn't fetch a picture", "Will try again at the next change."
			if errors.Is(err, errNoSources) {
				msg, sub = "No sources selected", "Turn up at least one picture source."
			}
			a.refreshStatusText(msg, sub, false)
		}
		return
	}
	log.Printf("wallpaper: %s (%s) %s", shown.Title, shown.Source, shown.PageURL)
	a.mu.Lock()
	a.current, a.offline = shown, offline
	a.mu.Unlock()
	if !offline {
		a.history.Push(shown)
		pruneCache(historyLen + 5)
	}
	a.refreshStatus(false)
	a.syncUI()
}

// ---- actions (menu and Settings window)

func (a *App) Next() {
	go a.change()
	a.kick()
}

func (a *App) Previous() {
	if a.busy.Load() {
		return
	}
	prev, ok := a.history.Back()
	if !ok {
		return
	}
	if err := setWallpaper(prev.Path); err != nil {
		log.Print("previous: ", err)
		return
	}
	a.mu.Lock()
	a.current = prev
	a.mu.Unlock()
	a.refreshStatus(false)
	a.kick() // give it a full interval
}

// Ban hides the current picture forever and moves on.
func (a *App) Ban() {
	cur := a.cur()
	if cur.ID == "" || strings.HasPrefix(cur.ID, "saved:") {
		return
	}
	a.fetcher.banned.Add(cur.ID)
	if h, err := fileHash(cur.Path); err == nil {
		a.fetcher.looks.Add(h) // also skips re-uploads of the same picture
	}
	a.history.Remove(cur.ID)
	if strings.HasPrefix(cur.Path, cacheDir) {
		_ = os.Remove(cur.Path)
	}
	log.Printf("banned: %s (%s)", cur.Title, cur.ID)
	a.Next()
}

func (a *App) OpenPage() {
	if u := a.cur().PageURL; u != "" {
		_ = exec.Command("open", u).Start()
	}
}

func (a *App) SaveCurrent() {
	cur := a.cur()
	if cur.Path == "" {
		return
	}
	_ = os.MkdirAll(savedDir, 0o755)
	name := strings.Trim(unsafeChars.ReplaceAllString(trunc(cur.Title, 80), "_"), "_")
	if name == "" {
		name = "wallpaper"
	}
	dst, saved := saveTarget(savedDir, name, filepath.Ext(cur.Path), cur.Path)
	if !saved {
		if err := copyFile(cur.Path, dst); err != nil {
			log.Print("save: ", err)
			return
		}
	}
	_ = exec.Command("open", "-R", dst).Start()
}

// saveTarget picks where to save src in dir without overwriting a different
// picture with the same title ("name.jpg", then "name_2.jpg", ...). saved is
// true when that exact picture is already there.
func saveTarget(dir, name, ext, src string) (path string, saved bool) {
	for i := 1; ; i++ {
		path = filepath.Join(dir, name+ext)
		if i > 1 {
			path = filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, false
		}
		if sameContents(src, path) {
			return path, true
		}
	}
}

func sameContents(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	return err == nil && bytes.Equal(x, y)
}

func (a *App) SetInterval(m int) {
	a.update(func(c *Config) { c.IntervalMin = m })
	a.kick()
}

func (a *App) SetWeight(name string, w int) {
	a.update(func(c *Config) { c.Weights[name] = min(max(w, 0), maxWeight) })
}

func (a *App) SetPaused(p bool) {
	a.update(func(c *Config) { c.Paused = p })
	if !p {
		a.mu.Lock()
		catchUp := a.missed
		a.missed = false
		a.mu.Unlock()
		if catchUp {
			go a.change()
		}
	}
	a.kick()
}

// SetPresence shows or hides the menu-bar icon and the Dock icon. At least
// one of them stays on so the app can always be reached.
func (a *App) SetPresence(menuBar, dock bool) {
	if !menuBar && !dock {
		menuBar = true
	}
	a.update(func(c *Config) { c.HideMenuBarIcon, c.HideDockIcon = !menuBar, !dock })
	settingsSetPresence(menuBar, dock)
}

func (a *App) SetKeepRunning(on bool) { a.update(func(c *Config) { c.KeepRunning = on }) }

func (a *App) SetSkipAI(skip bool) {
	a.update(func(c *Config) { c.DeviantArtAllowAI = !skip })
	a.fetcher.Reset("DeviantArt")
}

func (a *App) SetLogin(on bool) {
	if err := setLoginItem(on); err != nil {
		log.Print("login item: ", err)
	}
	if on && loginStatus() == LoginNeedsApproval {
		openLoginItemsSettings()
	}
	a.syncUI()
}

func (a *App) Quit() { quitApp() }

// SetDeviantKeys checks DeviantArt credentials, saves them on this Mac, and
// turns the source on. An empty secret keeps the saved one.
func (a *App) SetDeviantKeys(id, secret string) {
	_, saved, _ := a.deviantCreds()
	if secret == "" {
		secret = saved
	}
	a.fetcher.Reset("DeviantArt")
	if id == "" {
		deleteDeviantSecret()
		a.mu.Lock()
		a.daSecret, a.daSecretLoaded = "", true
		a.mu.Unlock()
		a.update(func(c *Config) { c.DeviantArtClientID = ""; c.Weights["DeviantArt"] = 0 })
		a.refreshDeviant("Keys removed. DeviantArt is off.")
		return
	}
	if secret == "" {
		a.refreshDeviant("❌ Enter the client secret too.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := daAuth.accessToken(ctx, id, secret); err != nil {
		log.Print(err)
		a.refreshDeviant("❌ " + err.Error())
		return
	}
	if err := saveDeviantSecret(secret); err != nil {
		a.refreshDeviant("❌ Couldn't save the keys: " + err.Error())
		return
	}
	a.mu.Lock()
	a.daSecret, a.daSecretLoaded = secret, true
	a.mu.Unlock()
	a.update(func(c *Config) {
		c.DeviantArtClientID = id
		if c.Weights["DeviantArt"] == 0 {
			c.Weights["DeviantArt"] = 3
		}
	})
	a.refreshDeviant("✅ Connected. Your keys are saved on this Mac.")
}

// ---- UI sync

func (a *App) update(f func(*Config)) {
	a.mu.Lock()
	f(&a.cfg)
	a.cfg.save()
	a.mu.Unlock()
	a.syncUI()
}

func (a *App) cur() Shown { a.mu.Lock(); defer a.mu.Unlock(); return a.current }

func (a *App) note() string {
	var notes []string
	if r := a.autoPauseReason(); r != "" && r != "Paused" {
		notes = append(notes, r+".")
	}
	a.mu.Lock()
	if a.offline || !a.sys.Online {
		notes = append(notes, "No internet: showing pictures already on this Mac.")
	}
	a.mu.Unlock()
	if loginStatus() == LoginNeedsApproval {
		notes = append(notes, "To start at login, allow Wallpaper Rotator in System Settings → General → Login Items.")
	}
	return strings.Join(notes, " ")
}

// syncUI pushes settings into both the menu and the Settings window.
func (a *App) syncUI() {
	a.mu.Lock()
	cfg := a.cfg
	weights := make([]int, len(a.sources))
	for i, s := range a.sources {
		weights[i] = a.cfg.Weights[s.Name()]
	}
	a.mu.Unlock()
	login := loginStatus()
	intervalIdx := -1
	for i, m := range intervals {
		if m == cfg.IntervalMin {
			intervalIdx = i
		}
	}
	if a.menu != nil {
		a.menu.sync(cfg, a.sources, login, a.autoPauseReason())
	}
	settingsSetState(intervalIdx, weights, cfg.Paused, login, cfg.KeepRunning, a.note())
}

func (a *App) refreshDeviant(status string) {
	a.mu.Lock()
	id, hasSecret, skip := a.cfg.DeviantArtClientID, a.daSecret != "" || (!a.daSecretLoaded && a.cfg.DeviantArtClientID != ""), !a.cfg.DeviantArtAllowAI
	a.mu.Unlock()
	settingsSetDeviantArt(id, hasSecret, skip, status)
}

func creditLine(s Shown) string {
	if s.Credit != "" && s.Credit != s.Source {
		return s.Credit + " · " + s.Source
	}
	return s.Source
}

// refreshStatus shows the current picture in the menu and window.
func (a *App) refreshStatus(busy bool) {
	cur := a.cur()
	if cur.ID == "" {
		title := "No picture yet"
		if r := a.autoPauseReason(); r != "" {
			title = r
		}
		a.refreshStatusText(title, "", busy)
		return
	}
	a.refreshStatusText(cur.Title, creditLine(cur), busy)
}

func (a *App) refreshStatusText(title, credit string, busy bool) {
	cur := a.cur()
	canBack := a.history.Len() > 1
	if a.menu != nil {
		a.menu.status(title, credit, busy, cur.ID != "", canBack)
	}
	settingsSetStatus(title, credit, cur.Path, cur.ID != "", busy, canBack)
}

// ---- helpers

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func intervalLabel(m int) string {
	switch m {
	case 1:
		return "1 minute"
	case 60:
		return "1 hour"
	}
	return fmt.Sprintf("%d minutes", m)
}
