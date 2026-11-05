package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	appName  = "Wallpaper Rotator"
	bundleID = "com.jamalxcode.wallpaperrotator"
	// Before the rename the app was "Wallpaper Changer"; its data is migrated once.
	legacyDirName  = "WallpaperChanger"
	legacyBundleID = "com.yoda.wallpaperchanger"
)

var (
	home       = must(os.UserHomeDir())
	supportDir = filepath.Join(home, "Library", "Application Support", "WallpaperRotator")
	cacheDir   = filepath.Join(supportDir, "images")
	configPath = filepath.Join(supportDir, "config.json")
	seenPath   = filepath.Join(supportDir, "seen.json")
	bannedPath = filepath.Join(supportDir, "banned.json")
	historyPth = filepath.Join(supportDir, "history.json")
	looksPath  = filepath.Join(supportDir, "fingerprints.json")
	logPath    = filepath.Join(home, "Library", "Logs", "WallpaperRotator.log")
	savedDir   = filepath.Join(home, "Pictures", "Wallpapers")
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Source weights: 0 = off, 1 = rarely … 5 = often.
const maxWeight = 5

var defaultWeights = map[string]int{
	"NASA": 3, "Wildlife": 3, "Wikimedia": 2, "Cleveland": 2, "Smithsonian": 2, "DeviantArt": 0,
}

type Config struct {
	IntervalMin int            `json:"interval_minutes"`
	Paused      bool           `json:"paused"`
	Weights     map[string]int `json:"weights"`
	// KeepRunning disables the automatic pause on Low Power Mode / phone hotspots.
	KeepRunning bool `json:"keep_running_on_low_power,omitempty"`
	// MainDisplayOnly changes only the display with the menu bar and leaves
	// any other displays (e.g. a small status screen) alone.
	MainDisplayOnly bool `json:"main_display_only,omitempty"`
	// Optional free key from https://api.data.gov/signup (DEMO_KEY is used otherwise).
	SmithsonianKey string `json:"smithsonian_api_key,omitempty"`
	// DeviantArt OAuth app (client credentials). The secret lives in the Keychain.
	DeviantArtClientID string `json:"deviantart_client_id,omitempty"`
	DeviantArtAllowAI  bool   `json:"deviantart_allow_ai,omitempty"`
	// Settings window opens automatically on the very first launch.
	WelcomeShown bool `json:"welcome_shown"`

	// Legacy fields, read for migration and then dropped.
	LegacySources map[string]bool `json:"sources,omitempty"`
	LegacySecret  string          `json:"deviantart_client_secret,omitempty"`
}

func loadConfig() Config { return loadConfigFrom(configPath) }

func loadConfigFrom(path string) Config {
	c := Config{IntervalMin: 4}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	c.normalize()
	return c
}

// normalize fills defaults and converts the old on/off "sources" map to weights.
func (c *Config) normalize() {
	if c.IntervalMin <= 0 {
		c.IntervalMin = 4
	}
	if c.Weights == nil {
		c.Weights = map[string]int{}
	}
	for name, def := range defaultWeights {
		if _, ok := c.Weights[name]; ok {
			continue
		}
		if on, ok := c.LegacySources[name]; ok {
			switch {
			case !on:
				c.Weights[name] = 0
			case def == 0:
				c.Weights[name] = 3 // was switched on by the user
			default:
				c.Weights[name] = def
			}
		} else {
			c.Weights[name] = def
		}
	}
	for name, w := range c.Weights {
		c.Weights[name] = min(max(w, 0), maxWeight)
	}
	c.LegacySources = nil
}

func (c Config) save() { c.saveTo(configPath) }

func (c Config) saveTo(path string) {
	b, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(path, b, 0o600)
	_ = os.Chmod(path, 0o600)
}

// migrateLegacy moves data from the old "Wallpaper Changer" install, once.
func migrateLegacy() {
	old := filepath.Join(home, "Library", "Application Support", legacyDirName)
	if _, err := os.Stat(supportDir); os.IsNotExist(err) {
		if _, err := os.Stat(old); err == nil {
			_ = os.Rename(old, supportDir)
		}
	}
	_ = os.Remove(filepath.Join(home, "Library", "Logs", "WallpaperChanger.log"))
	// The old app could write a LaunchAgent; login is now handled by SMAppService.
	_ = os.Remove(filepath.Join(home, "Library", "LaunchAgents", legacyBundleID+".plist"))
}

// IDSet is a persisted set of image IDs; limit > 0 keeps only the newest.
type IDSet struct {
	mu    sync.Mutex
	path  string
	limit int
	ids   []string
	set   map[string]bool
}

func loadIDSet(path string, limit int) *IDSet {
	s := &IDSet{path: path, limit: limit, set: map[string]bool{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.ids)
	}
	for _, id := range s.ids {
		s.set[id] = true
	}
	return s
}

func (s *IDSet) Has(id string) bool { s.mu.Lock(); defer s.mu.Unlock(); return s.set[id] }

func (s *IDSet) Add(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.set[id] {
		return
	}
	s.ids = append(s.ids, id)
	s.set[id] = true
	if s.limit > 0 && len(s.ids) > s.limit {
		for _, old := range s.ids[:len(s.ids)-s.limit] {
			delete(s.set, old)
		}
		s.ids = append([]string(nil), s.ids[len(s.ids)-s.limit:]...)
	}
	if s.path != "" {
		b, _ := json.Marshal(s.ids)
		_ = os.WriteFile(s.path, b, 0o644)
	}
}

// Shown is one wallpaper we displayed (persisted for "Previous").
type Shown struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Credit  string `json:"credit"`
	PageURL string `json:"page_url"`
	Source  string `json:"source"`
	Path    string `json:"path"`
}

const historyLen = 30 // also how many images stay in the cache

type History struct {
	mu    sync.Mutex
	path  string
	items []Shown // oldest first
}

func loadHistory(path string) *History {
	h := &History{path: path}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &h.items)
	}
	// drop entries whose file was pruned
	kept := h.items[:0]
	for _, it := range h.items {
		if _, err := os.Stat(it.Path); err == nil {
			kept = append(kept, it)
		}
	}
	h.items = kept
	return h
}

func (h *History) Push(s Shown) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = append(h.items, s)
	if len(h.items) > historyLen {
		h.items = append([]Shown(nil), h.items[len(h.items)-historyLen:]...)
	}
	h.saveLocked()
}

// Back removes the current (last) entry and returns the one before it.
func (h *History) Back() (Shown, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.items) < 2 {
		return Shown{}, false
	}
	h.items = h.items[:len(h.items)-1]
	h.saveLocked()
	return h.items[len(h.items)-1], true
}

// Remove drops every entry with this ID (used by "Never show again").
func (h *History) Remove(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	kept := h.items[:0]
	for _, it := range h.items {
		if it.ID != id {
			kept = append(kept, it)
		}
	}
	h.items = kept
	h.saveLocked()
}

func (h *History) Len() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.items) }

func (h *History) All() []Shown {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Shown(nil), h.items...)
}

func (h *History) saveLocked() {
	if h.path == "" {
		return
	}
	b, _ := json.Marshal(h.items)
	_ = os.WriteFile(h.path, b, 0o644)
}

// pruneCache keeps the newest `keep` images (names start with a timestamp).
func pruneCache(keep int) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	for i, e := range entries {
		if i >= keep {
			_ = os.Remove(filepath.Join(cacheDir, e.Name()))
		}
	}
}

// rotateLog keeps the log under ~2 MB, with one previous file.
func rotateLog(path string, max int64) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > max {
		_ = os.Rename(path, path+".1")
	}
}
