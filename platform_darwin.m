#import <AppKit/AppKit.h>
#import <Network/Network.h>
#import <Security/Security.h>
#import <ServiceManagement/ServiceManagement.h>
#import <Vision/Vision.h>
#include <stdlib.h>
#include "platform_darwin.h"
#include "_cgo_export.h"

static NSString *const kService = @"com.jamalxcode.wallpaperrotator";

// ------------------------------------------------------------------ wallpaper

static NSURL *gCurrent = nil; // latest wallpaper, re-applied on Space switches
static id gSpaceObs = nil, gScreenObs = nil;
static int gMainOnly = 0;

void wpSetMainDisplayOnly(int on) { gMainOnly = on; }

// Main thread only. Applies url to every screen (or just the main one),
// skipping ones already showing it.
static NSString *applyToAllScreens(NSURL *url) {
	NSString *errMsg = nil;
	NSWorkspace *ws = [NSWorkspace sharedWorkspace];
	NSDictionary *opts = @{
		NSWorkspaceDesktopImageScalingKey : @(NSImageScaleProportionallyUpOrDown),
		NSWorkspaceDesktopImageAllowClippingKey : @YES,
	};
	NSArray<NSScreen *> *screens = [NSScreen screens];
	// screens[0] is always the display with the menu bar
	if (gMainOnly && screens.count > 1) screens = @[ screens.firstObject ];
	for (NSScreen *s in screens) {
		if ([[ws desktopImageURLForScreen:s] isEqual:url]) continue;
		NSError *e = nil;
		if (![ws setDesktopImageURL:url forScreen:s options:opts error:&e]) {
			errMsg = e.localizedDescription ?: @"unknown error";
		}
	}
	return errMsg;
}

char *wpSetWallpaper(const char *path) {
	__block NSString *errMsg = nil;
	NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	void (^work)(void) = ^{
		gCurrent = url;
		errMsg = applyToAllScreens(url);
	};
	if ([NSThread isMainThread]) work();
	else dispatch_sync(dispatch_get_main_queue(), work);
	return errMsg ? strdup(errMsg.UTF8String) : NULL;
}

void wpStartSpaceWatcher(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gSpaceObs) return;
		void (^reapply)(NSNotification *) = ^(NSNotification *n) {
			// small delay: changes made mid-transition sometimes don't stick
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.3 * NSEC_PER_SEC)),
			               dispatch_get_main_queue(), ^{
				               if (gCurrent) applyToAllScreens(gCurrent);
			               });
		};
		gSpaceObs = [[[NSWorkspace sharedWorkspace] notificationCenter]
		    addObserverForName:NSWorkspaceActiveSpaceDidChangeNotification
		                object:nil
		                 queue:[NSOperationQueue mainQueue]
		            usingBlock:reapply];
		gScreenObs = [[NSNotificationCenter defaultCenter]
		    addObserverForName:NSApplicationDidChangeScreenParametersNotification
		                object:nil
		                 queue:[NSOperationQueue mainQueue]
		            usingBlock:reapply];
	});
}

// ------------------------------------------------------------------ system state

static int sLocked, sScreensAsleep, sLowPower, sExpensive, sOnline = 1;
static NSMutableArray *gMonitorObservers;
static nw_path_monitor_t gPathMonitor;

static void report(void) {
	goSystemState(sLocked, sScreensAsleep, sLowPower, sExpensive, sOnline);
}

void wpStartSystemMonitor(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gMonitorObservers) return;
		gMonitorObservers = [NSMutableArray array];
		NSNotificationCenter *ws = [[NSWorkspace sharedWorkspace] notificationCenter];
		NSDistributedNotificationCenter *dn = [NSDistributedNotificationCenter defaultCenter];
		NSOperationQueue *q = [NSOperationQueue mainQueue];
		void (^watch)(NSNotificationCenter *, NSString *, void (^)(void)) =
		    ^(NSNotificationCenter *c, NSString *name, void (^fn)(void)) {
			    [gMonitorObservers addObject:[c addObserverForName:name
			                                               object:nil
			                                                queue:q
			                                           usingBlock:^(NSNotification *n) {
				                                           fn();
				                                           report();
			                                           }]];
		    };
		watch(dn, @"com.apple.screenIsLocked", ^{ sLocked = 1; });
		watch(dn, @"com.apple.screenIsUnlocked", ^{ sLocked = 0; });
		watch(ws, NSWorkspaceScreensDidSleepNotification, ^{ sScreensAsleep = 1; });
		watch(ws, NSWorkspaceScreensDidWakeNotification, ^{ sScreensAsleep = 0; });
		watch(ws, NSWorkspaceWillSleepNotification, ^{ sScreensAsleep = 1; });
		watch(ws, NSWorkspaceDidWakeNotification, ^{ sScreensAsleep = 0; });
		watch([NSNotificationCenter defaultCenter], NSProcessInfoPowerStateDidChangeNotification, ^{
			sLowPower = NSProcessInfo.processInfo.lowPowerModeEnabled;
		});
		sLowPower = NSProcessInfo.processInfo.lowPowerModeEnabled;

		gPathMonitor = nw_path_monitor_create();
		nw_path_monitor_set_queue(gPathMonitor, dispatch_get_main_queue());
		nw_path_monitor_set_update_handler(gPathMonitor, ^(nw_path_t path) {
			sOnline = nw_path_get_status(path) == nw_path_status_satisfied;
			// "expensive" = cellular or a phone hotspot; "constrained" = Low Data Mode
			sExpensive = nw_path_is_expensive(path) || nw_path_is_constrained(path);
			report();
		});
		nw_path_monitor_start(gPathMonitor);
		report();
	});
}

// ------------------------------------------------------------------ login item

int wpLoginStatus(void) {
	if (@available(macOS 13.0, *)) {
		switch (SMAppService.mainAppService.status) {
		case SMAppServiceStatusEnabled: return 1;
		case SMAppServiceStatusRequiresApproval: return 2;
		case SMAppServiceStatusNotFound: return 3;
		default: return 0;
		}
	}
	return 3;
}

char *wpLoginSet(int on) {
	if (@available(macOS 13.0, *)) {
		NSError *e = nil;
		BOOL ok = on ? [SMAppService.mainAppService registerAndReturnError:&e]
		             : [SMAppService.mainAppService unregisterAndReturnError:&e];
		if (!ok) return strdup((e.localizedDescription ?: @"unknown error").UTF8String);
		return NULL;
	}
	return strdup("Start at login needs macOS 13 or later");
}

void wpOpenLoginItemsSettings(void) {
	if (@available(macOS 13.0, *)) {
		[SMAppService openSystemSettingsLoginItems];
	}
}

// ------------------------------------------------------------------ keychain

static NSMutableDictionary *kcQuery(const char *account) {
	return [@{
		(__bridge id)kSecClass : (__bridge id)kSecClassGenericPassword,
		(__bridge id)kSecAttrService : kService,
		(__bridge id)kSecAttrAccount : [NSString stringWithUTF8String:account],
	} mutableCopy];
}

int wpKeychainSet(const char *account, const char *secret) {
	NSData *data = [[NSString stringWithUTF8String:secret] dataUsingEncoding:NSUTF8StringEncoding];
	NSMutableDictionary *q = kcQuery(account);
	OSStatus st = SecItemUpdate((__bridge CFDictionaryRef)q,
	                            (__bridge CFDictionaryRef) @{(__bridge id)kSecValueData : data});
	if (st == errSecItemNotFound) {
		q[(__bridge id)kSecValueData] = data;
		q[(__bridge id)kSecAttrLabel] = @"Wallpaper Rotator: DeviantArt client secret";
		st = SecItemAdd((__bridge CFDictionaryRef)q, NULL);
	}
	return (int)st;
}

char *wpKeychainGet(const char *account) {
	NSMutableDictionary *q = kcQuery(account);
	q[(__bridge id)kSecReturnData] = @YES;
	q[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
	CFTypeRef out = NULL;
	if (SecItemCopyMatching((__bridge CFDictionaryRef)q, &out) != errSecSuccess || !out) return NULL;
	NSData *data = (__bridge_transfer NSData *)out;
	NSString *s = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
	return s ? strdup(s.UTF8String) : NULL;
}

void wpKeychainDelete(const char *account) {
	SecItemDelete((__bridge CFDictionaryRef)kcQuery(account));
}

// ------------------------------------------------------------------ Vision check

int wpVisionCheck(const char *path, double *humanArea, double *swimsuit, double *skull) {
	@autoreleasepool {
		*humanArea = *swimsuit = *skull = 0;
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		// Work on a ~1024px thumbnail: fast and plenty for this.
		CGImageSourceRef src = CGImageSourceCreateWithURL((__bridge CFURLRef)url, NULL);
		if (!src) return 0;
		NSDictionary *opts = @{
			(__bridge id)kCGImageSourceCreateThumbnailFromImageAlways : @YES,
			(__bridge id)kCGImageSourceThumbnailMaxPixelSize : @1024,
			(__bridge id)kCGImageSourceCreateThumbnailWithTransform : @YES,
		};
		CGImageRef img = CGImageSourceCreateThumbnailAtIndex(src, 0, (__bridge CFDictionaryRef)opts);
		CFRelease(src);
		if (!img) return 0;

		VNDetectHumanRectanglesRequest *humans = [VNDetectHumanRectanglesRequest new];
		humans.upperBodyOnly = NO;
		VNClassifyImageRequest *classify = [VNClassifyImageRequest new];
		VNImageRequestHandler *h = [[VNImageRequestHandler alloc] initWithCGImage:img options:@{}];
		NSError *e = nil;
		BOOL ok = [h performRequests:@[ humans, classify ] error:&e];
		CGImageRelease(img);
		if (!ok) return 0;

		double area = 0;
		for (VNHumanObservation *o in humans.results) {
			if (o.confidence < 0.5) continue;
			area += o.boundingBox.size.width * o.boundingBox.size.height;
		}
		*humanArea = MIN(area, 1.0);
		for (VNClassificationObservation *c in classify.results) {
			if ([c.identifier isEqualToString:@"swimsuit"]) *swimsuit = c.confidence;
			if ([c.identifier isEqualToString:@"skull"]) *skull = c.confidence;
		}
		return 1;
	}
}
