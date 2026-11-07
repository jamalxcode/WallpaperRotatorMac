package main

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "settings_darwin.h"
*/
import "C"

import (
	"log"
	"strconv"
	"strings"
	"unsafe"
)

// theApp receives actions from the Settings window.
var theApp *App

//export goLaunchKind
func goLaunchKind(atLogin C.int) {
	if atLogin != 0 {
		log.Print("started at login; Settings stays hidden")
	} else {
		log.Print("started by the user; showing Settings")
	}
}

//export goSettingsAction
func goSettingsAction(action, index, value C.int) {
	a := theApp
	if a == nil {
		return
	}
	i, on := int(index), value != 0
	// Called on the main thread: hand off so we never block the UI.
	go func() {
		switch action {
		case C.WCActionNext:
			a.Next()
		case C.WCActionPrevious:
			a.Previous()
		case C.WCActionBan:
			a.Ban()
		case C.WCActionOpenPage:
			a.OpenPage()
		case C.WCActionSave:
			a.SaveCurrent()
		case C.WCActionInterval:
			if i >= 0 && i < len(intervals) {
				a.SetInterval(intervals[i])
			}
		case C.WCActionWeight:
			if i >= 0 && i < len(a.sources) {
				a.SetWeight(a.sources[i].Name(), int(value))
			}
		case C.WCActionPause:
			a.SetPaused(on)
		case C.WCActionKeepRunning:
			a.SetKeepRunning(on)
		case C.WCActionLogin:
			a.SetLogin(on)
		case C.WCActionSkipAI:
			a.SetSkipAI(on)
		case C.WCActionQuit:
			a.Quit()
		case C.WCActionCheckUpdates:
			a.CheckForUpdates()
		case C.WCActionPresence:
			a.SetPresence(i != 0, on)
		}
	}()
}

//export goSettingsDeviantKeys
func goSettingsDeviantKeys(id, secret *C.char) {
	a := theApp
	if a == nil {
		return
	}
	i, s := C.GoString(id), C.GoString(secret) // copy before returning to Objective-C
	go a.SetDeviantKeys(i, s)
}

func withCString(s string, f func(*C.char)) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	f(cs)
}

func settingsInit(sourceLabels, intervalLabels []string) {
	withCString(strings.Join(sourceLabels, "\n"), func(src *C.char) {
		withCString(strings.Join(intervalLabels, "\n"), func(iv *C.char) {
			C.wcInit(src, iv)
		})
	})
	C.wcInstallReopenHandlers()
}

// showSettings opens the window on the settings tab used last; showMain on the current picture.
func showSettings() { C.wcShow(-1) }
func showMain()     { C.wcShow(0) }

func settingsSetPresence(menuBar, dock bool) { C.wcSetPresence(cbool(menuBar), cbool(dock)) }

// watchLaunch must run on the main thread before systray.Run.
func watchLaunch() { C.wcWatchLaunch() }

func settingsSetStatus(title, credit, imagePath string, hasImage, busy, canGoBack bool) {
	withCString(title, func(t *C.char) {
		withCString(credit, func(c *C.char) {
			withCString(imagePath, func(p *C.char) {
				C.wcSetStatus(t, c, p, cbool(hasImage), cbool(busy), cbool(canGoBack))
			})
		})
	})
}

func settingsSetState(intervalIndex int, weights []int, paused bool, login LoginStatus, keepRunning bool, note string) {
	parts := make([]string, len(weights))
	for i, w := range weights {
		parts[i] = strconv.Itoa(w)
	}
	withCString(strings.Join(parts, ","), func(ws *C.char) {
		withCString(note, func(n *C.char) {
			C.wcSetState(C.int(intervalIndex), ws, cbool(paused), C.int(login), cbool(keepRunning), n)
		})
	})
}

func settingsSetDeviantArt(id string, hasSecret, skipAI bool, status string) {
	withCString(id, func(ci *C.char) {
		withCString(status, func(st *C.char) {
			C.wcSetDeviantArt(ci, cbool(hasSecret), cbool(skipAI), st)
		})
	})
}

func settingsSetDeviantStatus(status string) {
	withCString(status, func(st *C.char) { C.wcSetDeviantStatus(st) })
}

func askRunningInstanceToShowSettings() { C.wcPostShowToRunningInstance() }

// alert shows a modal alert from a background goroutine and returns the
// 1-based index of the button pressed.
func alert(title, message string, buttons ...string) int {
	cs := make([]*C.char, 3)
	for i := 0; i < 3 && i < len(buttons); i++ {
		cs[i] = C.CString(buttons[i])
		defer C.free(unsafe.Pointer(cs[i]))
	}
	var r C.int
	withCString(title, func(t *C.char) {
		withCString(message, func(m *C.char) {
			r = C.wcAlert(t, m, cs[0], cs[1], cs[2])
		})
	})
	return int(r)
}

func takeCString(p *C.char) string {
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// appVersion is the bundle's version, or "0" when not running from the app.
func appVersion() string {
	if v := takeCString(C.wcAppVersion()); v != "" {
		return v
	}
	return "0"
}

func bundlePath() string { return takeCString(C.wcBundlePath()) }
