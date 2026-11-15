package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The menu-bar menu shows the last few wallpapers as a grid. Clicking one
// opens it (and the others) in Preview; ⌥-clicking puts it back on the desktop.

const recentCount = 9

var previewDir = filepath.Join(os.TempDir(), "WallpaperRotator Preview")

// recent returns up to recentCount pictures still on disk, newest first.
func (a *App) recent() []Shown {
	all := a.history.All()
	var out []Shown
	for i := len(all) - 1; i >= 0 && len(out) < recentCount; i-- {
		if _, err := os.Stat(all[i].Path); err == nil {
			out = append(out, all[i])
		}
	}
	return out
}

func (a *App) refreshRecent() {
	r := a.recent()
	a.mu.Lock()
	a.recentShown = r
	a.mu.Unlock()
	paths := make([]string, len(r))
	titles := make([]string, len(r))
	for i, s := range r {
		paths[i] = s.Path
		titles[i] = s.Title
		if c := creditLine(s); c != "" {
			titles[i] += " · " + c
		}
		titles[i] = strings.Join(strings.Fields(titles[i]), " ") // one line
	}
	setRecentGrid(paths, titles)
}

// recentAt returns the picture the grid showed at index i.
func (a *App) recentAt(i int) (Shown, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if i < 0 || i >= len(a.recentShown) {
		return Shown{}, false
	}
	return a.recentShown[i], true
}

// OpenRecentInPreview opens picture i in Preview, followed by the other
// recent pictures, under readable names.
func (a *App) OpenRecentInPreview(i int) {
	first, ok := a.recentAt(i)
	if !ok {
		return
	}
	a.mu.Lock()
	order := []Shown{first}
	for k, s := range a.recentShown {
		if k != i {
			order = append(order, s)
		}
	}
	a.mu.Unlock()

	_ = os.RemoveAll(previewDir)
	if err := os.MkdirAll(previewDir, 0o700); err != nil {
		log.Print("preview: ", err)
		return
	}
	used := map[string]bool{}
	args := []string{"-a", "Preview"}
	for _, s := range order {
		dst := filepath.Join(previewDir, previewName(s, used))
		if err := linkOrCopy(s.Path, dst); err != nil {
			log.Print("preview: ", err)
			continue
		}
		args = append(args, dst)
	}
	if len(args) == 2 {
		return
	}
	if err := exec.Command("/usr/bin/open", args...).Run(); err != nil {
		log.Print("preview: ", err)
	}
}

// previewName turns a picture's title into a unique, safe file name.
func previewName(s Shown, used map[string]bool) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '/', ':', '\\', 0:
			return '-'
		}
		return r
	}, strings.TrimSpace(s.Title))
	name = strings.TrimLeft(name, ".")
	if r := []rune(name); len(r) > 80 {
		name = strings.TrimSpace(string(r[:80]))
	}
	if name == "" {
		name = "Wallpaper"
	}
	ext := strings.ToLower(filepath.Ext(s.Path))
	if ext == "" {
		ext = ".jpg"
	}
	out := name + ext
	for n := 2; used[strings.ToLower(out)]; n++ {
		out = fmt.Sprintf("%s (%d)%s", name, n, ext)
	}
	used[strings.ToLower(out)] = true
	return out
}

// linkOrCopy hard-links src to dst (so the picture survives the cache being
// pruned while Preview has it open), copying if linking isn't possible.
func linkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ShowRecent puts recent picture i back on the desktop.
func (a *App) ShowRecent(i int) {
	s, ok := a.recentAt(i)
	if !ok || a.busy.Load() {
		return
	}
	if err := setWallpaper(s.Path); err != nil {
		log.Print("show recent: ", err)
		return
	}
	a.history.MoveToEnd(s.ID)
	a.mu.Lock()
	a.current = s
	a.mu.Unlock()
	a.refreshStatus(false)
	a.kick() // give it a full interval
}
