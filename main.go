// Wallpaper Rotator: a macOS menu-bar app that changes the desktop wallpaper
// every few minutes with public-domain pictures (and optionally DeviantArt).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"fyne.io/systray"
)

func main() {
	once := flag.Bool("once", false, "change the wallpaper once and exit (no menu bar icon)")
	dry := flag.Bool("dry", false, "with --once: download only, don't set the wallpaper")
	only := flag.String("source", "", "with --once: use only this source (NASA, Wildlife, Wikimedia, Cleveland, Smithsonian, DeviantArt)")
	checkFile := flag.String("check", "", "run the on-device image check on a file and print the result")
	flag.Parse()

	migrateLegacy()
	_ = os.MkdirAll(cacheDir, 0o755)
	rotateLog(logPath, 2<<20)
	if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, lf))
	}

	if *checkFile != "" {
		r, ok := visionCheck(*checkFile)
		fmt.Printf("ran=%v people=%.0f%% swimsuit=%.2f skull=%.2f\n", ok, r.HumanArea*100, r.Swimsuit, r.Skull)
		for _, kind := range []string{KindSpace, KindNature, KindArt} {
			fmt.Printf("  as %-6s → %v\n", kind, orOK(visionVerdict(kind, r)))
		}
		return
	}

	a := NewApp()
	if *once {
		runOnce(a, *only, *dry)
		return
	}

	// single instance: if already running, ask that copy to show its Settings window
	lock, err := os.OpenFile(filepath.Join(supportDir, "app.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err == nil && syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Print("already running; asking it to show Settings")
		askRunningInstanceToShowSettings()
		return
	}

	theApp = a
	systray.Run(a.onReady, func() {})
}

func (a *App) onReady() {
	a.menu = buildMenu(a)
	var srcLabels, ivLabels []string
	for _, s := range a.sources {
		srcLabels = append(srcLabels, sourceLabels[s.Name()])
	}
	for _, m := range intervals {
		ivLabels = append(ivLabels, intervalLabel(m))
	}
	settingsInit(srcLabels, ivLabels)
	a.syncUI()
	a.refreshStatus(false)
	a.mu.Lock()
	hasKeys := a.cfg.DeviantArtClientID != ""
	firstRun := !a.cfg.WelcomeShown
	a.cfg.WelcomeShown = true
	a.cfg.save()
	a.mu.Unlock()
	if hasKeys {
		a.refreshDeviant("Keys saved (secret in Keychain).")
	} else {
		a.refreshDeviant("Not connected.")
	}
	if firstRun {
		showSettings()
	}

	watchSpaces()
	go a.Run()
}

func runOnce(a *App, only string, dry bool) {
	weights := a.weights()
	if only != "" {
		weights = map[string]int{}
		for _, s := range a.sources {
			if strings.EqualFold(s.Name(), only) {
				weights[s.Name()] = 1
			}
		}
	}
	img, path, err := a.fetcher.Fetch(context.Background(), a.sources, weights)
	if err == nil && !dry {
		err = setWallpaper(path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n  %s · %s [%s]\n  %s\n  %s\n", img.Title, img.Credit, img.Source, img.Kind, img.PageURL, path)
}

func orOK(err error) string {
	if err == nil {
		return "OK"
	}
	return err.Error()
}
