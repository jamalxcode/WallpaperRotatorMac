package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework AppKit -framework Network -framework Security -framework ServiceManagement -framework Vision -framework ImageIO
#include <stdlib.h>
#include "platform_darwin.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"strconv"
	"unsafe"
)

// Keep main() on the main OS thread so AppKit calls work in --once mode too.
func init() { runtime.LockOSThread() }

func cErr(e *C.char) error {
	if e == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(e))
	return errors.New(C.GoString(e))
}

func setWallpaper(path string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	return cErr(C.wpSetWallpaper(cs))
}

func watchSpaces() { C.wpStartSpaceWatcher() }

// SystemState is what macOS tells us about whether changing wallpapers makes sense.
type SystemState struct {
	Locked, ScreensAsleep, LowPower, Expensive, Online bool
}

var onSystemState func(SystemState)

//export goSystemState
func goSystemState(locked, asleep, lowPower, expensive, online C.int) {
	st := SystemState{locked != 0, asleep != 0, lowPower != 0, expensive != 0, online != 0}
	if f := onSystemState; f != nil {
		go f(st) // called on the main thread; don't block it
	}
}

func startSystemMonitor(f func(SystemState)) {
	onSystemState = f
	C.wpStartSystemMonitor()
}

type LoginStatus int

const (
	LoginOff LoginStatus = iota
	LoginOn
	LoginNeedsApproval
	LoginUnavailable
)

func loginStatus() LoginStatus   { return LoginStatus(C.wpLoginStatus()) }
func openLoginItemsSettings()    { C.wpOpenLoginItemsSettings() }
func setLoginItem(on bool) error { return cErr(C.wpLoginSet(cbool(on))) }

func keychainSet(account, secret string) error {
	ca, cs := C.CString(account), C.CString(secret)
	defer C.free(unsafe.Pointer(ca))
	defer C.free(unsafe.Pointer(cs))
	if st := C.wpKeychainSet(ca, cs); st != 0 {
		return errors.New("Keychain error " + strconv.Itoa(int(st)))
	}
	return nil
}

func keychainGet(account string) string {
	ca := C.CString(account)
	defer C.free(unsafe.Pointer(ca))
	p := C.wpKeychainGet(ca)
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

func keychainDelete(account string) {
	ca := C.CString(account)
	defer C.free(unsafe.Pointer(ca))
	C.wpKeychainDelete(ca)
}

// visionCheck runs Apple Vision on an image file. ok=false if it couldn't run.
func visionCheck(path string) (VisionResult, bool) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	var area, swim, skull C.double
	if C.wpVisionCheck(cp, &area, &swim, &skull) == 0 {
		return VisionResult{}, false
	}
	return VisionResult{HumanArea: float64(area), Swimsuit: float64(swim), Skull: float64(skull)}, true
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
