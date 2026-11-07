package main

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.2.3", "1.2.2", true},
		{"v1.10.0", "1.9.9", true},
		{"2.0", "1.99.99", true},
		{"1.2.2", "1.2.2", false},
		{"1.2.1", "1.2.2", false},
		{"1.2.2", "0", true},
		{"1.3.0-beta", "1.2.9", true},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestReleaseHighlights(t *testing.T) {
	body := "## What's new\n- **Settings…** in the menu\n- Dock `menu`\n\nDownload it.\n- not this"
	want := "• Settings… in the menu\n• Dock menu"
	if got := releaseHighlights(body); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestZipURL(t *testing.T) {
	var r release
	r.Assets = append(r.Assets, struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}{Name: "WallpaperRotator-1.2.2.zip", URL: "versioned"})
	if r.zipURL() != "versioned" {
		t.Error("should fall back to the versioned zip")
	}
	r.Assets = append(r.Assets, struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}{Name: "WallpaperRotator.zip", URL: "fixed"})
	if r.zipURL() != "fixed" {
		t.Error("should prefer the fixed-name zip")
	}
}
