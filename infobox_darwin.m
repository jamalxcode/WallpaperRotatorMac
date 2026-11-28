// Desktop info box: a borderless window in the middle of the main display,
// just above the wallpaper (below desktop icons and every app window). It
// draws the clocks and the month itself and gets weather and prices as JSON
// from Go (infobox.go).
#import <Cocoa/Cocoa.h>
#include "settings_darwin.h"

void wcInfoBoxChecked(int on); // settings_darwin.m

// Everything is drawn in this design size and scaled to the window.
static const CGFloat kW = 828, kH = 574, kPad = 28;
// Share of the display's width the box takes, and the panel's opacity.
static const CGFloat kScreenShare = 0.44, kPanelAlpha = 0.22;

// Parts of the box that open something when clicked.
typedef NS_ENUM(NSInteger, WCLink) { WCLinkClock, WCLinkWeather, WCLinkHeat, WCLinkCalendar };

@interface WCInfoView : NSView
@property(copy) NSDictionary *data;
// Click areas from the last draw, in design coordinates: @[ @(WCLink), NSValue(rect) ].
@property(copy) NSArray *links;
@end

static void updateLinkWindows(void);

@implementation WCInfoView
- (BOOL)isFlipped { return YES; }

static NSColor *primary(void) { return NSColor.whiteColor; }
static NSColor *secondary(void) { return [NSColor colorWithWhite:0.9 alpha:1]; } // solid, just a little greyer
static NSColor *upColor(void) { return [NSColor colorWithSRGBRed:0.42 green:0.66 blue:1.0 alpha:1]; }
static NSColor *downColor(void) { return [NSColor colorWithSRGBRed:0.96 green:0.70 blue:0.29 alpha:1]; }

static NSFont *font(CGFloat size, NSFontWeight w) { return [NSFont monospacedDigitSystemFontOfSize:size weight:w]; }

// A soft dark shadow under every character so text stands out on bright or busy wallpapers.
static NSShadow *textShadow(void) {
	static NSShadow *s;
	if (!s) {
		s = [NSShadow new];
		s.shadowColor = [NSColor colorWithWhite:0 alpha:0.95];
		s.shadowOffset = NSMakeSize(0, -1.5); // down (shadow offsets ignore the flipped view)
		s.shadowBlurRadius = 4;
	}
	return s;
}

static NSDictionary *attrs(NSFont *f, NSColor *c, CGFloat kern) {
	return @{
		NSFontAttributeName : f,
		NSForegroundColorAttributeName : c,
		NSKernAttributeName : @(kern),
		NSShadowAttributeName : textShadow()
	};
}

// Draws s with its top at y; align -1 left of x, 0 centred on x, 1 right of x. Returns its width.
static CGFloat text(NSString *s, CGFloat x, CGFloat y, int align, NSDictionary *a) {
	if (!s.length) return 0;
	NSSize sz = [s sizeWithAttributes:a];
	CGFloat left = align < 0 ? x : align == 0 ? x - sz.width / 2 : x - sz.width;
	[s drawAtPoint:NSMakePoint(left, y) withAttributes:a];
	return sz.width;
}

// Values from the JSON, ignoring nulls and wrong types.
static NSString *str(NSDictionary *d, NSString *k) {
	id v = d[k];
	return [v isKindOfClass:NSString.class] ? v : @"";
}

static void rule(CGFloat y) {
	[NSGraphicsContext saveGraphicsState];
	[textShadow() set];
	[[NSColor colorWithWhite:1 alpha:0.55] setFill];
	NSRectFillUsingOperation(NSMakeRect(kPad, y, kW - 2 * kPad, 1), NSCompositingOperationSourceOver);
	[NSGraphicsContext restoreGraphicsState];
}

static NSString *timeIn(NSTimeZone *tz, NSDate *now) {
	NSDateFormatter *f = [NSDateFormatter new];
	f.locale = [NSLocale localeWithLocaleIdentifier:@"en_GB"];
	f.timeZone = tz;
	f.dateFormat = @"HH:mm";
	return [f stringFromDate:now];
}

- (void)drawRect:(NSRect)dirty {
	NSDate *now = [NSDate date];
	NSDictionary *d = self.data ?: @{};

	NSBezierPath *panel = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(0, 0, kW, kH) xRadius:26 yRadius:26];
	[[NSColor colorWithSRGBRed:0.07 green:0.075 blue:0.09 alpha:kPanelAlpha] setFill];
	[panel fill];

	// ---- Local time, date and weather
	NSMutableArray *links = [NSMutableArray array];
	void (^link)(WCLink, NSRect) = ^(WCLink kind, NSRect r) {
		[links addObject:@[ @(kind), [NSValue valueWithRect:NSInsetRect(r, -8, -4)] ]];
	};
	CGFloat timeW =
	    text(timeIn(NSTimeZone.localTimeZone, now), kPad - 2, 18, -1, attrs(font(46, NSFontWeightSemibold), primary(), 0.5));
	NSDateFormatter *df = [NSDateFormatter new];
	df.locale = [NSLocale localeWithLocaleIdentifier:@"en_GB"];
	df.dateFormat = @"EEE d MMM yyyy";
	NSString *city = str(d, @"city");
	NSString *date = [df stringFromDate:now].uppercaseString;
	CGFloat dateW = text([NSString stringWithFormat:@"%@ · %@", city, date], kPad, 72, -1,
	                     attrs(font(17, NSFontWeightRegular), secondary(), 1));
	link(WCLinkClock, NSMakeRect(kPad - 2, 20, MAX(timeW, dateW) + 2, 76));

	NSString *temp = str(d, @"temp"), *range = str(d, @"range"), *icon = str(d, @"icon");
	CGFloat right = kW - kPad;
	CGFloat tempW = temp.length ? text(temp, right, 24, 1, attrs(font(32, NSFontWeightSemibold), primary(), 0)) : 0;
	CGFloat rangeW = range.length ? text(range, right, 70, 1, attrs(font(17, NSFontWeightRegular), secondary(), 0)) : 0;
	if (icon.length) {
		NSImageSymbolConfiguration *cfg = [[NSImageSymbolConfiguration configurationWithPointSize:26 weight:NSFontWeightRegular]
		    configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[
			    [NSColor colorWithSRGBRed:0.98 green:0.78 blue:0.35 alpha:1]
		    ]]];
		NSImage *img = [[NSImage imageWithSystemSymbolName:icon accessibilityDescription:nil] imageWithSymbolConfiguration:cfg];
		if (img) {
			[NSGraphicsContext saveGraphicsState];
			[textShadow() set];
			CGFloat iconRight = right - MAX(rangeW, 120) - 22;
			link(WCLinkWeather, NSMakeRect(iconRight - img.size.width, 24, right - iconRight + img.size.width, 70));
			[img drawInRect:NSMakeRect(iconRight - img.size.width, 44 - img.size.height / 2, img.size.width, img.size.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
			[NSGraphicsContext restoreGraphicsState];
		}
	}
	if (temp.length && !icon.length) link(WCLinkWeather, NSMakeRect(right - MAX(tempW, rangeW), 24, MAX(tempW, rangeW), 70));
	rule(106);

	// ---- World clocks
	NSArray *clocks = @[
		@[ @"NEW YORK", @"America/New_York" ], @[ @"LONDON", @"Europe/London" ], @[ @"SINGAPORE", @"Asia/Singapore" ],
		@[ @"TOKYO", @"Asia/Tokyo" ]
	];
	CGFloat colW = (kW - 2 * kPad) / clocks.count;
	for (NSUInteger i = 0; i < clocks.count; i++) {
		CGFloat cx = kPad + colW * (i + 0.5);
		text(clocks[i][0], cx, 128, 0, attrs(font(16, NSFontWeightRegular), secondary(), 1));
		text(timeIn([NSTimeZone timeZoneWithName:clocks[i][1]], now), cx, 152, 0,
		     attrs(font(25, NSFontWeightSemibold), primary(), 0.5));
	}
	link(WCLinkClock, NSMakeRect(kPad, 126, kW - 2 * kPad, 58));
	rule(205);

	// ---- Month
	NSCalendar *cal = [NSCalendar calendarWithIdentifier:NSCalendarIdentifierGregorian];
	cal.firstWeekday = 1;
	NSDateComponents *c = [cal components:NSCalendarUnitYear | NSCalendarUnitMonth | NSCalendarUnitDay fromDate:now];
	df.dateFormat = @"MMMM yyyy";
	text([df stringFromDate:now].uppercaseString, kPad, 226, -1, attrs(font(17, NSFontWeightMedium), primary(), 1));
	CGFloat calW = 455, cellW = calW / 7, rowH = 27.5;
	NSArray *days = @[ @"S", @"M", @"T", @"W", @"T", @"F", @"S" ];
	for (int i = 0; i < 7; i++)
		text(days[i], kPad + cellW * (i + 0.5), 255, 0, attrs(font(15, NSFontWeightRegular), secondary(), 0));
	NSDateComponents *first = [c copy];
	first.day = 1;
	NSDate *firstDate = [cal dateFromComponents:first];
	NSInteger offset = [cal component:NSCalendarUnitWeekday fromDate:firstDate] - 1;
	NSInteger count = [cal rangeOfUnit:NSCalendarUnitDay inUnit:NSCalendarUnitMonth forDate:now].length;
	for (NSInteger day = 1; day <= count; day++) {
		NSInteger slot = offset + day - 1, col = slot % 7, row = slot / 7;
		CGFloat cx = kPad + cellW * (col + 0.5), y = 283 + rowH * row;
		BOOL today = day == c.day;
		if (today) {
			[[NSColor colorWithSRGBRed:0.30 green:0.50 blue:0.78 alpha:0.95] setFill];
			[[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(cx - cellW / 2 + 2, y - 4, cellW - 4, 28) xRadius:7 yRadius:7] fill];
		}
		text([NSString stringWithFormat:@"%ld", (long)day], cx, y, 0,
		     attrs(font(16, today ? NSFontWeightBold : NSFontWeightRegular), primary(), 0));
	}

	NSInteger rows = (offset + count + 6) / 7;
	link(WCLinkCalendar, NSMakeRect(kPad, 222, calW, 283 + rowH * rows - 222));

	// ---- Markets (from heat.sala.company)
	CGFloat mx = 503, mr = kW - kPad;
	text(@"MARKETS", mx, 226, -1, attrs(font(17, NSFontWeightMedium), primary(), 1));
	text(@"DAY %", mr, 226, 1, attrs(font(17, NSFontWeightRegular), secondary(), 1));
	NSArray *markets = [d[@"markets"] isKindOfClass:NSArray.class] ? d[@"markets"] : @[];
	for (NSUInteger i = 0; i < markets.count && i < 7; i++) {
		NSDictionary *m = markets[i];
		if (![m isKindOfClass:NSDictionary.class]) continue;
		CGFloat y = 256 + 30 * i;
		text(str(m, @"sym"), mx, y, -1, attrs(font(17, NSFontWeightMedium), primary(), 0.5));
		if (![m[@"ok"] boolValue]) {
			text(@"—", mr, y, 1, attrs(font(17, NSFontWeightRegular), secondary(), 0));
			continue;
		}
		double chg = [m[@"chg"] doubleValue];
		BOOL up = chg >= 0;
		NSString *pct = [NSString stringWithFormat:@"%@ %@%.2f%%", up ? @"▲" : @"▼", up ? @"+" : @"−", fabs(chg)];
		CGFloat pctW = text(pct, mr, y, 1, attrs(font(17, NSFontWeightRegular), up ? upColor() : downColor(), 0));
		text(str(m, @"price"), mr - MAX(112, pctW + 16), y + 1, 1, attrs(font(15, NSFontWeightRegular), secondary(), 0));
	}
	rule(478);

	// ---- Footer
	text(str(d, @"status"), kPad, 494, -1, attrs(font(16, NSFontWeightRegular), secondary(), 0));
	NSString *updated = str(d, @"updated");
	CGFloat heatW =
	    text(updated.length ? [NSString stringWithFormat:@"Prices %@ · heat.sala.company", updated] : @"Loading prices…", mr,
	         494, 1, attrs(font(16, NSFontWeightRegular), secondary(), 0));
	link(WCLinkHeat, NSMakeRect(mr - heatW, 494, heatW, 22));

	// ---- The wallpaper on the desktop now: a picture icon, its title, then its credit, cut short with … to fit
	NSString *picture = str(d, @"picture"), *credit = str(d, @"credit");
	if (picture.length || credit.length) {
		CGFloat y = 526, x = kPad;
		NSImageSymbolConfiguration *pcfg = [[NSImageSymbolConfiguration configurationWithPointSize:15 weight:NSFontWeightRegular]
		    configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[ secondary() ]]];
		NSImage *pic = [[NSImage imageWithSystemSymbolName:@"photo" accessibilityDescription:nil] imageWithSymbolConfiguration:pcfg];
		if (pic) {
			[NSGraphicsContext saveGraphicsState];
			[textShadow() set];
			[pic drawInRect:NSMakeRect(x, y + 11 - pic.size.height / 2, pic.size.width, pic.size.height)
			       fromRect:NSZeroRect
			      operation:NSCompositingOperationSourceOver
			       fraction:1
			 respectFlipped:YES
			          hints:nil];
			[NSGraphicsContext restoreGraphicsState];
			x += pic.size.width + 10;
		}
		NSMutableParagraphStyle *ps = [NSMutableParagraphStyle new];
		ps.lineBreakMode = NSLineBreakByTruncatingTail;
		NSMutableDictionary *ta = [attrs(font(16, NSFontWeightMedium), primary(), 0) mutableCopy];
		ta[NSParagraphStyleAttributeName] = ps;
		NSMutableDictionary *ca = [attrs(font(15, NSFontWeightRegular), secondary(), 0) mutableCopy];
		ca[NSParagraphStyleAttributeName] = ps;
		// The credit is always shown in full; a long title is the part cut short.
		NSString *tail = credit.length ? [NSString stringWithFormat:@"%@%@", picture.length ? @"  ·  " : @"", credit] : @"";
		CGFloat room = kW - kPad - x;
		CGFloat tailW = MIN([tail sizeWithAttributes:ca].width, room * 0.6);
		CGFloat titleW = MIN([picture sizeWithAttributes:ta].width, room - tailW);
		if (picture.length)
			[picture drawWithRect:NSMakeRect(x, y, titleW, 24)
			              options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingTruncatesLastVisibleLine
			           attributes:ta];
		if (tail.length)
			[tail drawWithRect:NSMakeRect(x + titleW, y + 1, room - titleW, 24)
			           options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingTruncatesLastVisibleLine
			        attributes:ca];
	}

	if (![links isEqualToArray:self.links ?: @[]]) {
		self.links = links;
		dispatch_async(dispatch_get_main_queue(), ^{ updateLinkWindows(); }); // not while drawing
	}
}
@end

// ---- Click areas
// The box itself lets every click through to the desktop. Each link gets its
// own small, invisible window just above the desktop icons, so only those
// spots catch the mouse (with a pointing hand and a soft highlight).

// Opens an app the way Finder does, so one that is already running gets a "reopen" and shows its window.
static void reopenApp(NSString *bundleID) {
	[NSTask launchedTaskWithExecutableURL:[NSURL fileURLWithPath:@"/usr/bin/open"]
	                            arguments:@[ @"-b", bundleID ]
	                                error:nil
	                   terminationHandler:nil];
}

static void openLink(WCLink kind) {
	NSWorkspace *ws = NSWorkspace.sharedWorkspace;
	NSURL *app = nil;
	switch (kind) {
	case WCLinkHeat:
		[ws openURL:[NSURL URLWithString:@"https://heat.sala.company/"]];
		return;
	case WCLinkCalendar: {
		// Smart Cal, or Google Calendar on the web without it. Smart Cal is a menu-bar app: just
		// activating it shows nothing, so it's "opened again" like a Finder click (open -b), which
		// makes it show its drop-down under the menu-bar icon.
		NSString *smartCal = @"company.sala.personalcalendar";
		if (![ws URLForApplicationWithBundleIdentifier:smartCal]) {
			[ws openURL:[NSURL URLWithString:@"https://calendar.google.com/"]];
			return;
		}
		BOOL running = [NSRunningApplication runningApplicationsWithBundleIdentifier:smartCal].count > 0;
		reopenApp(smartCal);
		if (!running) // the first open only launches it; open it again once it's up to show the drop-down
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{ reopenApp(smartCal); });
		return;
	}
	case WCLinkWeather:
		app = [ws URLForApplicationWithBundleIdentifier:@"com.apple.weather"];
		break;
	case WCLinkClock:
		app = [ws URLForApplicationWithBundleIdentifier:@"com.apple.clock"];
		if (!app) { // no Clock app before macOS 14: Date & Time settings instead
			[ws openURL:[NSURL URLWithString:@"x-apple.systempreferences:com.apple.Date-Time-Settings.extension"]];
			return;
		}
		break;
	}
	if (app)
		[ws openApplicationAtURL:app configuration:[NSWorkspaceOpenConfiguration configuration] completionHandler:nil];
}

@interface WCLinkView : NSView
@property WCLink kind;
@property BOOL hover, pressed;
@end

@implementation WCLinkView
- (BOOL)acceptsFirstMouse:(NSEvent *)e { return YES; }
- (void)drawRect:(NSRect)r {
	// Nearly clear: the window must have some paint to catch clicks.
	[[NSColor colorWithWhite:1 alpha:self.pressed ? 0.22 : self.hover ? 0.12 : 0.01] setFill];
	[[NSBezierPath bezierPathWithRoundedRect:self.bounds xRadius:10 yRadius:10] fill];
}
- (void)updateTrackingAreas {
	for (NSTrackingArea *t in self.trackingAreas) [self removeTrackingArea:t];
	[self addTrackingArea:[[NSTrackingArea alloc]
	                          initWithRect:NSZeroRect
	                               options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect
	                                 owner:self
	                              userInfo:nil]];
	[super updateTrackingAreas];
}
- (void)mouseEntered:(NSEvent *)e {
	self.hover = YES;
	[NSCursor.pointingHandCursor set];
	self.needsDisplay = YES;
}
- (void)mouseExited:(NSEvent *)e {
	self.hover = self.pressed = NO;
	[NSCursor.arrowCursor set];
	self.needsDisplay = YES;
}
- (void)mouseDown:(NSEvent *)e {
	self.pressed = YES;
	self.needsDisplay = YES;
}
- (void)mouseUp:(NSEvent *)e {
	BOOL inside = NSPointInRect([self convertPoint:e.locationInWindow fromView:nil], self.bounds);
	self.pressed = NO;
	self.needsDisplay = YES;
	if (inside) openLink(self.kind);
}
@end

static NSWindow *gInfoWindow;
static WCInfoView *gInfoView;
static NSTimer *gInfoTimer;
static id gScreenObserver, gWakeObserver;
static NSMutableArray<NSPanel *> *gLinkWindows;

static NSPanel *makeLinkWindow(void) {
	NSPanel *p = [[NSPanel alloc] initWithContentRect:NSZeroRect
	                                        styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
	                                          backing:NSBackingStoreBuffered
	                                            defer:YES];
	p.releasedWhenClosed = NO;
	p.opaque = NO;
	p.backgroundColor = NSColor.clearColor;
	p.hasShadow = NO;
	p.level = CGWindowLevelForKey(kCGDesktopIconWindowLevelKey) + 1; // above Finder's desktop, below apps
	p.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
	                       NSWindowCollectionBehaviorIgnoresCycle | NSWindowCollectionBehaviorFullScreenNone;
	p.excludedFromWindowsMenu = YES;
	p.hidesOnDeactivate = NO;
	p.becomesKeyOnlyIfNeeded = YES;
	p.contentView = [[WCLinkView alloc] initWithFrame:NSZeroRect];
	return p;
}

// Puts one click window over each link drawn in the box (hides them with the box).
static void updateLinkWindows(void) {
	if (!gLinkWindows) gLinkWindows = [NSMutableArray array];
	NSArray *links = gInfoWindow.visible ? gInfoView.links : @[];
	for (NSUInteger i = 0; i < links.count; i++) {
		if (i == gLinkWindows.count) [gLinkWindows addObject:makeLinkWindow()];
		NSPanel *p = gLinkWindows[i];
		NSRect inWindow = [gInfoView convertRect:[links[i][1] rectValue] toView:nil];
		[p setFrame:[gInfoWindow convertRectToScreen:inWindow] display:NO];
		((WCLinkView *)p.contentView).kind = [links[i][0] integerValue];
		p.contentView.needsDisplay = YES;
		[p orderFront:nil];
	}
	for (NSUInteger i = links.count; i < gLinkWindows.count; i++) [gLinkWindows[i] orderOut:nil];
}

static void placeInfoWindow(void) {
	NSScreen *screen = NSScreen.screens.firstObject; // the display with the menu bar
	if (!screen || !gInfoWindow) return;
	NSRect f = screen.frame;
	CGFloat w = MIN(kW, round(f.size.width * kScreenShare)), h = round(w * kH / kW);
	NSRect r = NSMakeRect(round(NSMidX(f) - w / 2), round(NSMidY(f) - h / 2), w, h);
	[gInfoWindow setFrame:r display:YES];
	[gInfoView setBoundsSize:NSMakeSize(kW, kH)];
	gInfoView.needsDisplay = YES;
	updateLinkWindows(); // the box moved or changed size
}

// Redraws on each new minute so the clocks stay right.
static void scheduleTick(void) {
	[gInfoTimer invalidate];
	NSTimeInterval t = NSDate.date.timeIntervalSinceReferenceDate;
	NSDate *next = [NSDate dateWithTimeIntervalSinceReferenceDate:floor(t / 60) * 60 + 60.2];
	gInfoTimer = [[NSTimer alloc] initWithFireDate:next interval:0 repeats:NO block:^(NSTimer *_) {
		gInfoView.needsDisplay = YES;
		scheduleTick();
	}];
	gInfoTimer.tolerance = 0.5;
	[NSRunLoop.mainRunLoop addTimer:gInfoTimer forMode:NSRunLoopCommonModes];
}

static void makeInfoWindow(void) {
	gInfoWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, kW, kH)
	                                          styleMask:NSWindowStyleMaskBorderless
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	gInfoWindow.releasedWhenClosed = NO;
	gInfoWindow.opaque = NO;
	gInfoWindow.backgroundColor = NSColor.clearColor;
	gInfoWindow.hasShadow = NO;
	gInfoWindow.ignoresMouseEvents = YES;
	gInfoWindow.level = CGWindowLevelForKey(kCGDesktopWindowLevelKey) + 1;
	gInfoWindow.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
	                                 NSWindowCollectionBehaviorIgnoresCycle | NSWindowCollectionBehaviorFullScreenNone;
	gInfoWindow.excludedFromWindowsMenu = YES;
	gInfoWindow.title = @"Info Box";
	gInfoView = [[WCInfoView alloc] initWithFrame:NSMakeRect(0, 0, kW, kH)];
	gInfoWindow.contentView = gInfoView;
	gScreenObserver = [NSNotificationCenter.defaultCenter addObserverForName:NSApplicationDidChangeScreenParametersNotification
	                                                                  object:nil
	                                                                   queue:NSOperationQueue.mainQueue
	                                                              usingBlock:^(NSNotification *n) { placeInfoWindow(); }];
	gWakeObserver = [NSWorkspace.sharedWorkspace.notificationCenter addObserverForName:NSWorkspaceDidWakeNotification
	                                                                            object:nil
	                                                                             queue:NSOperationQueue.mainQueue
	                                                                        usingBlock:^(NSNotification *n) {
		                                                                        if (gInfoWindow.visible) {
			                                                                        gInfoView.needsDisplay = YES;
			                                                                        scheduleTick();
		                                                                        }
	                                                                        }];
}

void wcSetInfoBox(int on, const char *data) {
	NSData *json = [NSData dataWithBytes:data length:strlen(data)];
	dispatch_async(dispatch_get_main_queue(), ^{
		wcInfoBoxChecked(on);
		if (!on) {
			[gInfoTimer invalidate];
			gInfoTimer = nil;
			[gInfoWindow orderOut:nil];
			updateLinkWindows();
			return;
		}
		if (!gInfoWindow) makeInfoWindow();
		NSDictionary *d = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if ([d isKindOfClass:NSDictionary.class]) gInfoView.data = d;
		if (!gInfoWindow.visible) {
			placeInfoWindow();
			[gInfoWindow orderFront:nil];
			scheduleTick();
			updateLinkWindows();
		}
		gInfoView.needsDisplay = YES;
	});
}
