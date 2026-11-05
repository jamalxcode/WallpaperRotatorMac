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

	// single instance
	lock, err := os.OpenFile(filepath.Join(supportDir, "app.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err == nil && syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Print("already running; exiting")
		return
	}

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
	for _, m := range intervals {
		label := fmt.Sprintf("%d minutes", m)
		if m == 1 {
			label = "1 minute"
		} else if m == 60 {
			label = "1 hour"
		}
		a.mIntervals[m] = mEvery.AddSubMenuItemCheckbox(label, "", m == a.cfg.IntervalMin)
	}
	mSrc := systray.AddMenuItem("Sources", "")
	a.mSources = map[string]*systray.MenuItem{}
	for _, s := range allSources {
		label := sourceLabels[s.Name()]
		a.mSources[s.Name()] = mSrc.AddSubMenuItemCheckbox(label, "", a.cfg.Sources[s.Name()])
	}
	a.mLogin = systray.AddMenuItemCheckbox("Start at Login", "", loginEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")

	a.refreshPause()
	a.mOpen.Disable()
	a.mSave.Disable()

	watchSpaces()
	go a.handleClicks(mQuit)
	go a.loop()
}

func (a *App) handleClicks(mQuit *systray.MenuItem) {
	for m, item := range a.mIntervals {
		go func() {
			for range item.ClickedCh {
				a.mu.Lock()
				a.cfg.IntervalMin = m
				a.cfg.save()
				a.mu.Unlock()
				for mm, it := range a.mIntervals {
					if mm == m {
						it.Check()
					} else {
						it.Uncheck()
					}
				}
				a.kick()
			}
		}()
	}
	for name, item := range a.mSources {
		go func() {
			for range item.ClickedCh {
				a.mu.Lock()
				on := !a.cfg.Sources[name]
				a.cfg.Sources[name] = on
				a.cfg.save()
				a.mu.Unlock()
				if on {
					item.Check()
				} else {
					item.Uncheck()
				}
			}
		}()
	}
	for {
		select {
		case <-a.mNext.ClickedCh:
			go a.change()
			a.kick()
		case <-a.mPause.ClickedCh:
			a.mu.Lock()
			a.cfg.Paused = !a.cfg.Paused
			a.cfg.save()
			a.mu.Unlock()
			a.refreshPause()
			a.kick()
		case <-a.mOpen.ClickedCh:
			if u := a.cur().PageURL; u != "" {
				_ = exec.Command("open", u).Start()
			}
		case <-a.mSave.ClickedCh:
			a.saveCurrent()
		case <-a.mLogin.ClickedCh:
			on := !a.mLogin.Checked()
			if err := setLogin(on); err != nil {
				log.Print("login item: ", err)
			} else if on {
				a.mLogin.Check()
			} else {
				a.mLogin.Uncheck()
			}
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) kick() {
	select {
	case a.reset <- struct{}{}:
	default:
	}
}

func (a *App) cur() Image { a.mu.Lock(); defer a.mu.Unlock(); return a.current }

func (a *App) refreshPause() {
	a.mu.Lock()
	p := a.cfg.Paused
	a.mu.Unlock()
	if p {
		a.mPause.SetTitle("Resume")
		systray.SetTooltip("Wallpaper Changer (paused)")
	} else {
		a.mPause.SetTitle("Pause")
		systray.SetTooltip("Wallpaper Changer")
	}
}

func (a *App) loop() {
	if !a.cfg.Paused {
		a.change()
	} else {
		a.mInfo.SetTitle("Paused")
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

	prevTitle := a.cur().Title
	a.mInfo.SetTitle("Finding a new picture…")
	a.mNext.Disable()
	defer a.mNext.Enable()

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
		if prevTitle != "" {
			a.mInfo.SetTitle(trunc(prevTitle, 55))
		} else {
			a.mInfo.SetTitle("Couldn't fetch a picture (will retry)")
		}
		return
	}
	log.Printf("wallpaper: %s (%s) %s", img.Title, img.Source, img.PageURL)

	a.mu.Lock()
	a.current, a.curPath = img, path
	a.mu.Unlock()
	a.mInfo.SetTitle(trunc(img.Title, 55))
	credit := img.Source
	if img.Credit != "" && img.Credit != img.Source {
		credit = img.Credit + " · " + img.Source
	}
	a.mCredit.SetTitle(trunc(credit, 55))
	a.mOpen.Enable()
	a.mSave.Enable()
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
