package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
)

var allSources []Source

var sourceLabels = map[string]string{
	"NASA":        "NASA (space & Earth)",
	"Wikimedia":   "Wikimedia Commons (featured photos)",
	"Cleveland":   "Cleveland Museum of Art (paintings & prints)",
	"Smithsonian": "Smithsonian American Art (landscapes)",
}

var intervals = []int{1, 4, 10, 30, 60}

type App struct {
	mu      sync.Mutex
	cfg     Config
	seen    *Seen
	current Image
	curPath string
	busy    atomic.Bool
	reset   chan struct{}

	mInfo, mCredit, mNext, mPause, mOpen, mSave, mLogin *systray.MenuItem
	mIntervals                                          map[int]*systray.MenuItem
	mSources                                            map[string]*systray.MenuItem
}

func main() {
	once := flag.Bool("once", false, "change the wallpaper once and exit (no menu bar icon)")
	dry := flag.Bool("dry", false, "with --once: download only, don't set the wallpaper")
	only := flag.String("source", "", "with --once: use only this source (NASA, Wikimedia, Cleveland, Smithsonian)")
	flag.Parse()

	_ = os.MkdirAll(cacheDir, 0o755)
	logPath := filepath.Join(must(os.UserHomeDir()), "Library", "Logs", "WallpaperChanger.log")
	if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, lf))
	}

	a := &App{cfg: loadConfig(), seen: loadSeen(), reset: make(chan struct{}, 1)}
	allSources = []Source{NASA{}, Wikimedia{}, ClevelandArt{}, Smithsonian{Key: a.cfg.SmithsonianKey}}

	if *once {
		srcs := a.enabledSources()
		if *only != "" {
			srcs = nil
			for _, s := range allSources {
				if strings.EqualFold(s.Name(), *only) {
					srcs = append(srcs, s)
				}
			}
		}
		img, path, err := fetchWallpaper(context.Background(), srcs, a.seen)
		if err == nil && !*dry {
			err = setWallpaper(path)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("%s\n  %s · %s\n  %s\n  %s\n", img.Title, img.Credit, img.Source, img.PageURL, path)
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

func (a *App) enabledSources() []Source {
	var out []Source
	for _, s := range allSources {
		if a.cfg.Sources[s.Name()] {
			out = append(out, s)
		}
	}
	return out
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

func (a *App) onReady() {
	systray.SetTemplateIcon(menuIcon(), menuIcon())
	systray.SetTooltip("Wallpaper Changer")

	a.mInfo = systray.AddMenuItem("Starting…", "")
	a.mInfo.Disable()
	a.mCredit = systray.AddMenuItem("", "")
	a.mCredit.Disable()
	systray.AddSeparator()
	a.mNext = systray.AddMenuItem("Next Wallpaper", "Change now")
	a.mPause = systray.AddMenuItem("Pause", "Stop changing automatically")
	a.mOpen = systray.AddMenuItem("Open Image Page", "See the source and license")
	a.mSave = systray.AddMenuItem("Save to Pictures", "Copy to ~/Pictures/Wallpapers")
	systray.AddSeparator()

	mEvery := systray.AddMenuItem("Change Every", "")
	a.mIntervals = map[int]*systray.MenuItem{}
	var intervalLabels []string
	for _, m := range intervals {
		a.mIntervals[m] = mEvery.AddSubMenuItemCheckbox(intervalLabel(m), "", false)
		intervalLabels = append(intervalLabels, intervalLabel(m))
	}
	mSrc := systray.AddMenuItem("Sources", "")
	a.mSources = map[string]*systray.MenuItem{}
	var srcLabels []string
	for _, s := range allSources {
		a.mSources[s.Name()] = mSrc.AddSubMenuItemCheckbox(sourceLabels[s.Name()], "", false)
		srcLabels = append(srcLabels, sourceLabels[s.Name()])
	}
	a.mLogin = systray.AddMenuItemCheckbox("Start at Login", "", false)
	mSettings := systray.AddMenuItem("Settings…", "Open the Settings window")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")

	a.mOpen.Disable()
	a.mSave.Disable()

	settingsInit(srcLabels, intervalLabels)
	a.syncUI()
	a.setStatus("Starting…", "", false)

	a.mu.Lock()
	firstRun := !a.cfg.WelcomeShown
	a.cfg.WelcomeShown = true
	a.cfg.save()
	a.mu.Unlock()
	if firstRun {
		showSettings()
	}

	watchSpaces()
	go a.handleClicks(mSettings, mQuit)
	go a.loop()
}

func (a *App) handleClicks(mSettings, mQuit *systray.MenuItem) {
	for m, item := range a.mIntervals {
		go func() {
			for range item.ClickedCh {
				a.setInterval(m)
			}
		}()
	}
	for name, item := range a.mSources {
		go func() {
			for range item.ClickedCh {
				a.mu.Lock()
				on := !a.cfg.Sources[name]
				a.mu.Unlock()
				a.setSource(name, on)
			}
		}()
	}
	for {
		select {
		case <-a.mNext.ClickedCh:
			a.next()
		case <-a.mPause.ClickedCh:
			a.mu.Lock()
			p := !a.cfg.Paused
			a.mu.Unlock()
			a.setPaused(p)
		case <-a.mOpen.ClickedCh:
			a.openPage()
		case <-a.mSave.ClickedCh:
			a.saveCurrent()
		case <-a.mLogin.ClickedCh:
			a.setLoginOn(!loginEnabled())
		case <-mSettings.ClickedCh:
			showSettings()
		case <-mQuit.ClickedCh:
			a.quit()
			return
		}
	}
}

// ---- actions shared by the menu and the Settings window

func (a *App) next() {
	go a.change()
	a.kick()
}

func (a *App) openPage() {
	if u := a.cur().PageURL; u != "" {
		_ = exec.Command("open", u).Start()
	}
}

func (a *App) setInterval(m int) {
	a.mu.Lock()
	a.cfg.IntervalMin = m
	a.cfg.save()
	a.mu.Unlock()
	a.syncUI()
	a.kick()
}

func (a *App) setSource(name string, on bool) {
	a.mu.Lock()
	a.cfg.Sources[name] = on
	a.cfg.save()
	a.mu.Unlock()
	a.syncUI()
}

func (a *App) setPaused(p bool) {
	a.mu.Lock()
	a.cfg.Paused = p
	a.cfg.save()
	a.mu.Unlock()
	a.syncUI()
	a.kick()
}

func (a *App) setLoginOn(on bool) {
	if err := setLogin(on); err != nil {
		log.Print("login item: ", err)
	}
	a.syncUI()
}

func (a *App) quit() { systray.Quit() }

// syncUI pushes settings into both the menu and the Settings window.
func (a *App) syncUI() {
	a.mu.Lock()
	interval, paused := a.cfg.IntervalMin, a.cfg.Paused
	enabled := map[string]bool{}
	for k, v := range a.cfg.Sources {
		enabled[k] = v
	}
	a.mu.Unlock()
	login := loginEnabled()

	intervalIdx := -1
	for i, m := range intervals {
		if m == interval {
			intervalIdx = i
			a.mIntervals[m].Check()
		} else {
			a.mIntervals[m].Uncheck()
		}
	}
	mask := 0
	for i, s := range allSources {
		if enabled[s.Name()] {
			mask |= 1 << i
			a.mSources[s.Name()].Check()
		} else {
			a.mSources[s.Name()].Uncheck()
		}
	}
	if login {
		a.mLogin.Check()
	} else {
		a.mLogin.Uncheck()
	}
	if paused {
		a.mPause.SetTitle("Resume")
		systray.SetTooltip("Wallpaper Changer (paused)")
	} else {
		a.mPause.SetTitle("Pause")
		systray.SetTooltip("Wallpaper Changer")
	}
	settingsSetState(intervalIdx, mask, paused, login)
}

// setStatus shows what's on screen (or what's happening) in the menu and window.
func (a *App) setStatus(title, credit string, busy bool) {
	a.mInfo.SetTitle(trunc(title, 55))
	a.mCredit.SetTitle(trunc(credit, 55))
	if busy {
		a.mNext.Disable()
	} else {
		a.mNext.Enable()
	}
	a.mu.Lock()
	has := a.curPath != ""
	a.mu.Unlock()
	if has {
		a.mOpen.Enable()
		a.mSave.Enable()
	}
	settingsSetStatus(title, credit, has, busy)
}

func (a *App) kick() {
	select {
	case a.reset <- struct{}{}:
	default:
	}
}

func (a *App) cur() Image { a.mu.Lock(); defer a.mu.Unlock(); return a.current }

func creditLine(img Image) string {
	if img.Credit != "" && img.Credit != img.Source {
		return img.Credit + " · " + img.Source
	}
	return img.Source
}

func (a *App) loop() {
	a.mu.Lock()
	paused := a.cfg.Paused
	a.mu.Unlock()
	if !paused {
		a.change()
	} else {
		a.setStatus("Paused", "Untick “Pause” to start changing wallpapers.", false)
	}
	for {
		a.mu.Lock()
		d := time.Duration(a.cfg.IntervalMin) * time.Minute
		a.mu.Unlock()
		t := time.NewTimer(d)
		select {
		case <-t.C:
			a.mu.Lock()
			paused := a.cfg.Paused
			a.mu.Unlock()
			if !paused {
				a.change()
			}
		case <-a.reset:
			t.Stop()
		}
	}
}

func (a *App) change() {
	if !a.busy.CompareAndSwap(false, true) {
		return
	}
	defer a.busy.Store(false)

	prev := a.cur()
	a.setStatus("Finding a new picture…", "", true)

	a.mu.Lock()
	srcs := a.enabledSources()
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	img, path, err := fetchWallpaper(ctx, srcs, a.seen)
	if err == nil {
		err = setWallpaper(path)
	}
	if err != nil {
		log.Print("change failed: ", err)
		if prev.Title != "" {
			a.setStatus(prev.Title, creditLine(prev), false)
		} else if len(srcs) == 0 {
			a.setStatus("No sources selected", "Tick at least one picture source.", false)
		} else {
			a.setStatus("Couldn't fetch a picture", "Will try again at the next change.", false)
		}
		return
	}
	log.Printf("wallpaper: %s (%s) %s", img.Title, img.Source, img.PageURL)

	a.mu.Lock()
	a.current, a.curPath = img, path
	a.mu.Unlock()
	a.setStatus(img.Title, creditLine(img), false)
	pruneCache(keepInCache)
}

func (a *App) saveCurrent() {
	a.mu.Lock()
	img, src := a.current, a.curPath
	a.mu.Unlock()
	if src == "" {
		return
	}
	dir := filepath.Join(must(os.UserHomeDir()), "Pictures", "Wallpapers")
	_ = os.MkdirAll(dir, 0o755)
	name := unsafeChars.ReplaceAllString(trunc(img.Title, 80), "_")
	dst := filepath.Join(dir, strings.Trim(name, "_")+filepath.Ext(src))
	in, err := os.Open(src)
	if err != nil {
		log.Print(err)
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		log.Print(err)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		log.Print(err)
		return
	}
	_ = exec.Command("open", "-R", dst).Start()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
