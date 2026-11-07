package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Updates come from the GitHub releases of this repository.
const (
	releasesPage = "https://github.com/jamalxcode/WallpaperRotatorMac/releases"
	latestAPI    = "https://api.github.com/repos/jamalxcode/WallpaperRotatorMac/releases/latest"
	updateAsset  = "WallpaperRotator.zip"
	maxUpdate    = 200 << 20
)

type release struct {
	Tag     string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// zipURL picks the app download: the fixed-name copy, or any WallpaperRotator-*.zip.
func (r release) zipURL() string {
	fallback := ""
	for _, a := range r.Assets {
		if a.Name == updateAsset {
			return a.URL
		}
		if fallback == "" && strings.HasPrefix(a.Name, "WallpaperRotator-") && strings.HasSuffix(a.Name, ".zip") {
			fallback = a.URL
		}
	}
	return fallback
}

// versionParts turns "v1.2.10" into [1 2 10]; anything unparsable counts as 0.
func versionParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

// isNewer reports whether version latest is newer than current.
func isNewer(latest, current string) bool {
	l, c := versionParts(latest), versionParts(current)
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func fetchLatestRelease(ctx context.Context) (release, error) {
	var r release
	req, err := http.NewRequestWithContext(ctx, "GET", latestAPI, nil)
	if err != nil {
		return r, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return r, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r)
	return r, err
}

var checkingUpdate atomic.Bool

// CheckForUpdates asks GitHub for the latest release and offers to install it.
func (a *App) CheckForUpdates() {
	if !checkingUpdate.CompareAndSwap(false, true) {
		return
	}
	defer checkingUpdate.Store(false)

	current := appVersion()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := fetchLatestRelease(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		alert("Couldn't check for updates", err.Error(), "OK")
		return
	}
	log.Printf("update check: running %s, latest %s", current, r.Tag)
	latest := strings.TrimPrefix(r.Tag, "v")
	if !isNewer(latest, current) {
		alert("You're up to date", fmt.Sprintf("Wallpaper Rotator %s is the latest version.", current), "OK")
		return
	}

	msg := fmt.Sprintf("Version %s is available. You have %s.", latest, current)
	if notes := releaseHighlights(r.Body); notes != "" {
		msg += "\n\n" + notes
	}
	zip := r.zipURL()
	if zip == "" {
		if alert("A new version is available", msg, "View on GitHub", "Later") == 1 {
			_ = exec.Command("open", r.HTMLURL).Start()
		}
		return
	}
	switch alert("A new version is available", msg, "Install and Restart", "View on GitHub", "Later") {
	case 1:
		if err := installUpdate(zip); err != nil {
			log.Printf("update failed: %v", err)
			if alert("The update didn't install", err.Error()+"\n\nYou can download it from GitHub instead.",
				"Open GitHub", "Cancel") == 1 {
				_ = exec.Command("open", releasesPage+"/latest").Start()
			}
		}
	case 2:
		_ = exec.Command("open", r.HTMLURL).Start()
	}
}

// releaseHighlights keeps the bullet points of the release notes' "What's new" list.
func releaseHighlights(body string) string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			if len(out) > 0 {
				break
			}
			continue
		}
		line = strings.NewReplacer("**", "", "`", "").Replace(line)
		out = append(out, "• "+strings.TrimPrefix(line, "- "))
		if len(out) == 6 {
			break
		}
	}
	return strings.Join(out, "\n")
}

// installUpdate downloads the release zip, checks it, swaps it in for the
// running app and restarts.
func installUpdate(zipURL string) error {
	appPath := bundlePath()
	if !strings.HasSuffix(appPath, ".app") {
		return errors.New("this copy isn't running from an app bundle")
	}
	dir, err := os.MkdirTemp("", "wallpaper-rotator-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	zipPath := filepath.Join(dir, updateAsset)
	if err := downloadTo(zipURL, zipPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	unpacked := filepath.Join(dir, "unpacked")
	if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, unpacked).CombinedOutput(); err != nil {
		return fmt.Errorf("couldn't unzip the update: %v %s", err, out)
	}
	newApp := filepath.Join(unpacked, "WallpaperRotator.app")
	id, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier",
		filepath.Join(newApp, "Contents", "Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(id)) != bundleID {
		return errors.New("the download isn't a Wallpaper Rotator app")
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", newApp).CombinedOutput(); err != nil {
		return fmt.Errorf("the download's signature is broken: %s", strings.TrimSpace(string(out)))
	}

	// Copy next to the current app first (same disk), then swap with renames.
	staged := appPath + ".update"
	old := appPath + ".old"
	_ = os.RemoveAll(staged)
	_ = os.RemoveAll(old)
	if out, err := exec.Command("/usr/bin/ditto", newApp, staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(staged)
		return fmt.Errorf("couldn't copy into %s: %v %s", filepath.Dir(appPath), err, out)
	}
	if err := os.Rename(appPath, old); err != nil {
		_ = os.RemoveAll(staged)
		return fmt.Errorf("couldn't replace the app: %w", err)
	}
	if err := os.Rename(staged, appPath); err != nil {
		_ = os.Rename(old, appPath)
		_ = os.RemoveAll(staged)
		return fmt.Errorf("couldn't replace the app: %w", err)
	}
	_ = os.RemoveAll(old)
	log.Printf("updated %s; restarting", appPath)

	// Start the new copy once this one has exited (it holds the single-instance lock).
	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; /usr/bin/open "$2"`
	cmd := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(os.Getpid()), appPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		alert("Update installed", "Open Wallpaper Rotator again to use the new version.", "OK")
	}
	quitApp()
	return nil
}

func downloadTo(u, path string) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s", resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxUpdate+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxUpdate {
		err = errors.New("download is too large")
	}
	return err
}
