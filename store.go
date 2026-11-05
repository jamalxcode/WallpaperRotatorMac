package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const bundleID = "com.yoda.wallpaperchanger"

var (
	supportDir = filepath.Join(must(os.UserHomeDir()), "Library", "Application Support", "WallpaperChanger")
	cacheDir   = filepath.Join(supportDir, "images")
	configPath = filepath.Join(supportDir, "config.json")
	seenPath   = filepath.Join(supportDir, "seen.json")
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

type Config struct {
	IntervalMin int             `json:"interval_minutes"`
	Paused      bool            `json:"paused"`
	Sources     map[string]bool `json:"sources"`
	// Optional free key from https://api.data.gov/signup (DEMO_KEY is used otherwise).
	SmithsonianKey string `json:"smithsonian_api_key,omitempty"`
	// DeviantArt OAuth app credentials (client credentials grant). Kept only here.
	DeviantArtClientID     string `json:"deviantart_client_id,omitempty"`
	DeviantArtClientSecret string `json:"deviantart_client_secret,omitempty"`
	DeviantArtAllowAI      bool   `json:"deviantart_allow_ai,omitempty"`
	// Settings window opens automatically on the very first launch.
	WelcomeShown bool `json:"welcome_shown"`
}

func loadConfig() Config {
	c := Config{IntervalMin: 4, Sources: map[string]bool{"NASA": true, "Wildlife": true, "Wikimedia": true, "Cleveland": true, "Smithsonian": true, "DeviantArt": false}}
	// json.Unmarshal merges into this map, so sources added later default to on.
	if b, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.IntervalMin <= 0 {
		c.IntervalMin = 4
	}
	return c
}

func (c Config) save() {
	b, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(configPath, b, 0o600) // may hold an API secret
	_ = os.Chmod(configPath, 0o600)
}

// Seen keeps recently used image IDs so we don't repeat.
type Seen struct {
	mu  sync.Mutex
	ids []string
	set map[string]bool
}

const seenCap = 2000

func loadSeen() *Seen {
	s := &Seen{set: map[string]bool{}}
	if b, err := os.ReadFile(seenPath); err == nil {
		_ = json.Unmarshal(b, &s.ids)
	}
	for _, id := range s.ids {
		s.set[id] = true
	}
	return s
}

func (s *Seen) Has(id string) bool { s.mu.Lock(); defer s.mu.Unlock(); return s.set[id] }

func (s *Seen) Add(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids = append(s.ids, id)
	s.set[id] = true
	if len(s.ids) > seenCap {
		for _, old := range s.ids[:len(s.ids)-seenCap] {
			delete(s.set, old)
		}
		s.ids = s.ids[len(s.ids)-seenCap:]
	}
	b, _ := json.Marshal(s.ids)
	_ = os.WriteFile(seenPath, b, 0o644)
}

// pruneCache keeps the newest `keep` images.
func pruneCache(keep int) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() }) // names start with unix time
	for i, e := range entries {
		if i >= keep {
			_ = os.Remove(filepath.Join(cacheDir, e.Name()))
		}
	}
}
