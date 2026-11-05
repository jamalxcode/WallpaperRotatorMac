package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fileTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestConfigMigratesSourcesToWeights(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := `{"interval_minutes": 10, "sources": {"NASA": false, "Wildlife": true, "DeviantArt": true},
	         "deviantart_client_secret": "s3cret"}`
	os.WriteFile(path, []byte(old), 0o644)

	c := loadConfigFrom(path)
	if c.IntervalMin != 10 {
		t.Errorf("interval = %d, want 10", c.IntervalMin)
	}
	want := map[string]int{"NASA": 0, "Wildlife": 3, "DeviantArt": 3, "Cleveland": 2}
	for k, w := range want {
		if c.Weights[k] != w {
			t.Errorf("weight[%s] = %d, want %d", k, c.Weights[k], w)
		}
	}
	if c.LegacySources != nil {
		t.Error("legacy sources map should be dropped after migration")
	}
	if c.LegacySecret != "s3cret" {
		t.Error("legacy secret should be readable so the app can move it to the Keychain")
	}

	c.LegacySecret = ""
	c.saveTo(path)
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("config permissions = %v, want 0600", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(path)
	if saved := string(b); strings.Contains(saved, "s3cret") || strings.Contains(saved, `"sources"`) {
		t.Errorf("saved config still has legacy fields: %s", saved)
	}
}

func TestConfigDefaultsAndClamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"interval_minutes": 0, "weights": {"NASA": 9, "Wikimedia": -2}}`), 0o644)
	c := loadConfigFrom(path)
	if c.IntervalMin != 4 {
		t.Errorf("interval default = %d, want 4", c.IntervalMin)
	}
	if c.Weights["NASA"] != maxWeight || c.Weights["Wikimedia"] != 0 {
		t.Errorf("weights not clamped: %v", c.Weights)
	}
	if c.Weights["DeviantArt"] != 0 {
		t.Error("DeviantArt should default to off")
	}
}

func TestIDSetLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s := loadIDSet(path, 3)
	for _, id := range []string{"a", "b", "c", "d", "d"} {
		s.Add(id)
	}
	if s.Has("a") || !s.Has("b") || !s.Has("d") {
		t.Errorf("limit not applied: %v", s.ids)
	}
	if reloaded := loadIDSet(path, 3); !reloaded.Has("c") || reloaded.Has("a") {
		t.Error("IDSet not persisted correctly")
	}
}

func TestHistory(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) Shown {
		p := filepath.Join(dir, n+".jpg")
		os.WriteFile(p, []byte("x"), 0o644)
		return Shown{ID: n, Path: p}
	}
	h := loadHistory(filepath.Join(dir, "history.json"))
	if _, ok := h.Back(); ok {
		t.Error("Back on empty history should fail")
	}
	h.Push(mk("1"))
	h.Push(mk("2"))
	h.Push(mk("3"))
	if prev, ok := h.Back(); !ok || prev.ID != "2" {
		t.Errorf("Back = %v %v, want 2", prev.ID, ok)
	}
	h.Remove("2")
	if h.Len() != 1 {
		t.Errorf("after Remove, Len = %d, want 1", h.Len())
	}
	os.Remove(filepath.Join(dir, "1.jpg"))
	if loadHistory(filepath.Join(dir, "history.json")).Len() != 0 {
		t.Error("entries whose file is gone should be dropped on load")
	}
	for i := 0; i < historyLen+5; i++ {
		h.Push(mk(string(rune('a' + i%26))))
	}
	if h.Len() != historyLen {
		t.Errorf("history not capped: %d", h.Len())
	}
}

func TestAutoPauseReason(t *testing.T) {
	cases := []struct {
		cfg  Config
		sys  SystemState
		want string
	}{
		{Config{}, SystemState{Online: true}, ""},
		{Config{Paused: true}, SystemState{}, "Paused"},
		{Config{}, SystemState{Locked: true}, "Paused while the screen is locked"},
		{Config{}, SystemState{ScreensAsleep: true}, "Paused while the display is asleep"},
		{Config{}, SystemState{LowPower: true}, "Paused: Low Power Mode is on"},
		{Config{KeepRunning: true}, SystemState{LowPower: true, Expensive: true}, ""},
		{Config{}, SystemState{Expensive: true}, "Paused: on a phone hotspot or Low Data Mode"},
	}
	for _, c := range cases {
		a := &App{cfg: c.cfg, sys: c.sys}
		if got := a.autoPauseReason(); got != c.want {
			t.Errorf("autoPauseReason(%+v, %+v) = %q, want %q", c.cfg, c.sys, got, c.want)
		}
	}
}

func TestRotateLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	os.WriteFile(p, make([]byte, 100), 0o644)
	rotateLog(p, 50)
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Error("big log should be rotated")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("rotated log should be moved away")
	}
}

func TestSaveTargetKeepsDifferentPictures(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(t.TempDir(), "a.jpg"), filepath.Join(t.TempDir(), "b.jpg")
	os.WriteFile(a, []byte("picture A"), 0o644)
	os.WriteFile(b, []byte("picture B"), 0o644)

	p, saved := saveTarget(dir, "Sunset", ".jpg", a)
	if saved || filepath.Base(p) != "Sunset.jpg" {
		t.Fatalf("first save = %s %v, want Sunset.jpg false", p, saved)
	}
	copyFile(a, p)
	if p, saved = saveTarget(dir, "Sunset", ".jpg", a); !saved || filepath.Base(p) != "Sunset.jpg" {
		t.Errorf("same picture again = %s %v, want Sunset.jpg true", p, saved)
	}
	p, saved = saveTarget(dir, "Sunset", ".jpg", b)
	if saved || filepath.Base(p) != "Sunset_2.jpg" {
		t.Fatalf("different picture, same title = %s %v, want Sunset_2.jpg false", p, saved)
	}
	copyFile(b, p)
	if got, _ := os.ReadFile(filepath.Join(dir, "Sunset.jpg")); string(got) != "picture A" {
		t.Error("first picture was overwritten")
	}
}
