package main

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "settings_darwin.h"
*/
import "C"

import (
	"strings"
	"unsafe"
)

// theApp receives actions from the Settings window.
var theApp *App

//export goSettingsAction
func goSettingsAction(action, index, on C.int) {
	a := theApp
	if a == nil {
		return
	}
	// Called on the main thread: hand off so we never block the UI.
	go func() {
		switch action {
		case C.WCActionNext:
			a.next()
		case C.WCActionOpenPage:
			a.openPage()
		case C.WCActionSave:
			a.saveCurrent()
		case C.WCActionInterval:
			if i := int(index); i >= 0 && i < len(intervals) {
				a.setInterval(intervals[i])
			}
		case C.WCActionSource:
			if i := int(index); i >= 0 && i < len(allSources) {
				a.setSource(allSources[i].Name(), on != 0)
			}
		case C.WCActionPause:
			a.setPaused(on != 0)
		case C.WCActionLogin:
			a.setLoginOn(on != 0)
		case C.WCActionQuit:
			a.quit()
		}
	}()
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

func showSettings() { C.wcShowSettings() }

func settingsSetStatus(title, credit string, hasImage, busy bool) {
	withCString(title, func(t *C.char) {
		withCString(credit, func(c *C.char) {
			C.wcSetStatus(t, c, cbool(hasImage), cbool(busy))
		})
	})
}

func settingsSetState(intervalIndex, sourcesMask int, paused, login bool) {
	C.wcSetState(C.int(intervalIndex), C.int(sourcesMask), cbool(paused), cbool(login))
}

func askRunningInstanceToShowSettings() { C.wcPostShowToRunningInstance() }

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
