package main

import (
	"fyne.io/systray"
)

// Menu is the menu-bar icon's menu. It only displays state and forwards
// clicks to App; App.syncUI / refreshStatus keep it current.
type Menu struct {
	info, credit, next, prev, ban, pause, open, save, login *systray.MenuItem
	intervals                                               map[int]*systray.MenuItem
	sources                                                 map[string]*systray.MenuItem
}

func quitApp() { systray.Quit() }

func buildMenu(a *App) *Menu {
	systray.SetTemplateIcon(menuIcon(), menuIcon())
	systray.SetTooltip(appName)

	m := &Menu{intervals: map[int]*systray.MenuItem{}, sources: map[string]*systray.MenuItem{}}
	m.info = systray.AddMenuItem("Starting…", "")
	m.info.Disable()
	m.credit = systray.AddMenuItem("", "")
	m.credit.Disable()
	systray.AddSeparator()
	m.next = systray.AddMenuItem("Next Wallpaper", "Change now")
	m.prev = systray.AddMenuItem("Previous Wallpaper", "Go back to the last one")
	m.ban = systray.AddMenuItem("Never Show This Again", "Hide this picture forever and move on")
	m.pause = systray.AddMenuItem("Pause", "Stop changing automatically")
	m.open = systray.AddMenuItem("Open Image Page", "See the source and license")
	m.save = systray.AddMenuItem("Save to Pictures", "Copy to ~/Pictures/Wallpapers")
	systray.AddSeparator()
	every := systray.AddMenuItem("Change Every", "")
	for _, min := range intervals {
		m.intervals[min] = every.AddSubMenuItemCheckbox(intervalLabel(min), "", false)
	}
	src := systray.AddMenuItem("Sources", "Use Settings… to set how often each appears")
	for _, s := range a.sources {
		m.sources[s.Name()] = src.AddSubMenuItemCheckbox(sourceLabels[s.Name()], "", false)
	}
	m.login = systray.AddMenuItemCheckbox("Start at Login", "", false)
	settings := systray.AddMenuItem("Settings…", "Open the Settings window")
	updates := systray.AddMenuItem("Check for Updates…", "See if a newer version is on GitHub")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	m.prev.Disable()
	m.ban.Disable()
	m.open.Disable()
	m.save.Disable()

	for min, item := range m.intervals {
		go func() {
			for range item.ClickedCh {
				a.SetInterval(min)
			}
		}()
	}
	for name, item := range m.sources {
		go func() {
			for range item.ClickedCh {
				// menu is on/off; the window has the full 0..5 scale
				w := 0
				if a.weights()[name] == 0 {
					w = max(defaultWeights[name], 3)
				}
				a.SetWeight(name, w)
			}
		}()
	}
	go func() {
		for {
			select {
			case <-m.next.ClickedCh:
				a.Next()
			case <-m.prev.ClickedCh:
				go a.Previous()
			case <-m.ban.ClickedCh:
				go a.Ban()
			case <-m.pause.ClickedCh:
				a.mu.Lock()
				p := !a.cfg.Paused
				a.mu.Unlock()
				go a.SetPaused(p)
			case <-m.open.ClickedCh:
				a.OpenPage()
			case <-m.save.ClickedCh:
				go a.SaveCurrent()
			case <-m.login.ClickedCh:
				go a.SetLogin(loginStatus() != LoginOn)
			case <-settings.ClickedCh:
				showSettings()
			case <-updates.ClickedCh:
				go a.CheckForUpdates()
			case <-quit.ClickedCh:
				a.Quit()
				return
			}
		}
	}()
	return m
}

func check(item *systray.MenuItem, on bool) {
	if on {
		item.Check()
	} else {
		item.Uncheck()
	}
}

func enable(item *systray.MenuItem, on bool) {
	if on {
		item.Enable()
	} else {
		item.Disable()
	}
}

func (m *Menu) sync(cfg Config, sources []Source, login LoginStatus, pauseReason string) {
	for min, item := range m.intervals {
		check(item, min == cfg.IntervalMin)
	}
	for _, s := range sources {
		check(m.sources[s.Name()], cfg.Weights[s.Name()] > 0)
	}
	check(m.login, login == LoginOn)
	if cfg.Paused {
		m.pause.SetTitle("Resume")
	} else {
		m.pause.SetTitle("Pause")
	}
	tip := appName
	if pauseReason != "" {
		tip += " (" + pauseReason + ")"
	}
	systray.SetTooltip(tip)
}

func (m *Menu) status(title, credit string, busy, hasImage, canBack bool) {
	m.info.SetTitle(trunc(title, 55))
	m.credit.SetTitle(trunc(credit, 55))
	enable(m.next, !busy)
	enable(m.prev, !busy && canBack)
	enable(m.ban, !busy && hasImage)
	enable(m.open, hasImage)
	enable(m.save, hasImage)
}
