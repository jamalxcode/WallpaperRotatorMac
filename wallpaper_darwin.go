package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>

static NSURL *gCurrent = nil;     // latest wallpaper, re-applied on Space switches
static id gSpaceObs = nil, gScreenObs = nil;

// Main thread only. Applies url to every screen of the current Space,
// skipping screens that already show it.
static NSString* applyToAllScreens(NSURL *url) {
	NSString *errMsg = nil;
	NSWorkspace *ws = [NSWorkspace sharedWorkspace];
	NSDictionary *opts = @{
		NSWorkspaceDesktopImageScalingKey: @(NSImageScaleProportionallyUpOrDown),
		NSWorkspaceDesktopImageAllowClippingKey: @YES,
	};
	for (NSScreen *s in [NSScreen screens]) {
		if ([[ws desktopImageURLForScreen:s] isEqual:url]) continue;
		NSError *e = nil;
		if (![ws setDesktopImageURL:url forScreen:s options:opts error:&e]) {
			errMsg = e.localizedDescription ?: @"unknown error";
		}
	}
	return errMsg;
}

// Sets the wallpaper on every connected screen (current Space) and remembers it
// so other Spaces get it when you switch to them.
// Returns NULL on success or a malloc'd error string.
static char* setWallpaperAllScreens(const char* path) {
	__block NSString *errMsg = nil;
	NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	void (^work)(void) = ^{
		gCurrent = url;
		errMsg = applyToAllScreens(url);
	};
	if ([NSThread isMainThread]) work(); else dispatch_sync(dispatch_get_main_queue(), work);
	return errMsg ? strdup(errMsg.UTF8String) : NULL;
}

// macOS only lets apps set the wallpaper of the active Space, so when the user
// switches Space (or plugs in a display) re-apply the latest image there.
static void startSpaceWatcher(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gSpaceObs) return;
		void (^reapply)(NSNotification *) = ^(NSNotification *n) {
			// small delay: changes made mid-transition sometimes don't stick
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.3 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
				if (gCurrent) applyToAllScreens(gCurrent);
			});
		};
		gSpaceObs = [[[NSWorkspace sharedWorkspace] notificationCenter]
			addObserverForName:NSWorkspaceActiveSpaceDidChangeNotification object:nil
			queue:[NSOperationQueue mainQueue] usingBlock:reapply];
		gScreenObs = [[NSNotificationCenter defaultCenter]
			addObserverForName:NSApplicationDidChangeScreenParametersNotification object:nil
			queue:[NSOperationQueue mainQueue] usingBlock:reapply];
	});
}
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

func setWallpaper(path string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	if e := C.setWallpaperAllScreens(cs); e != nil {
		defer C.free(unsafe.Pointer(e))
		return errors.New(C.GoString(e))
	}
	return nil
}

// Keep main() on the main OS thread so AppKit calls work in --once mode too.
func init() { runtime.LockOSThread() }

// watchSpaces re-applies the latest wallpaper whenever the active Space or
// display setup changes. Call once the app's run loop is up.
func watchSpaces() { C.startSpaceWatcher() }
