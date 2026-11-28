// Desktop info box: a borderless window in the middle of the main display,
// just above the wallpaper (below desktop icons and every app window). It
// draws the clocks and the month itself and gets weather and prices as JSON
// from Go (infobox.go).
#import <Cocoa/Cocoa.h>
#include "settings_darwin.h"

void wcInfoBoxChecked(int on); // settings_darwin.m

// Everything is drawn in this design size and scaled to the window.
static const CGFloat kW = 828, kH = 543, kPad = 28;
// Share of the display's width the box takes, and the panel's opacity.
static const CGFloat kScreenShare = 0.44, kPanelAlpha = 0.5;

@interface WCInfoView : NSView
@property(copy) NSDictionary *data;
@end

@implementation WCInfoView
- (BOOL)isFlipped { return YES; }

static NSColor *primary(void) { return [NSColor colorWithWhite:1 alpha:0.96]; }
static NSColor *secondary(void) { return [NSColor colorWithWhite:1 alpha:0.78]; }
static NSColor *upColor(void) { return [NSColor colorWithSRGBRed:0.42 green:0.66 blue:1.0 alpha:1]; }
static NSColor *downColor(void) { return [NSColor colorWithSRGBRed:0.96 green:0.70 blue:0.29 alpha:1]; }

static NSFont *font(CGFloat size, NSFontWeight w) { return [NSFont monospacedDigitSystemFontOfSize:size weight:w]; }

static NSDictionary *attrs(NSFont *f, NSColor *c, CGFloat kern) {
	return @{NSFontAttributeName : f, NSForegroundColorAttributeName : c, NSKernAttributeName : @(kern)};
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
	[[NSColor colorWithWhite:1 alpha:0.22] setFill];
	NSRectFill(NSMakeRect(kPad, y, kW - 2 * kPad, 1));
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
	text(timeIn(NSTimeZone.localTimeZone, now), kPad - 2, 18, -1, attrs(font(46, NSFontWeightSemibold), primary(), 0.5));
	NSDateFormatter *df = [NSDateFormatter new];
	df.locale = [NSLocale localeWithLocaleIdentifier:@"en_GB"];
	df.dateFormat = @"EEE d MMM yyyy";
	NSString *city = str(d, @"city");
	NSString *date = [df stringFromDate:now].uppercaseString;
	text([NSString stringWithFormat:@"%@ · %@", city, date], kPad, 72, -1, attrs(font(17, NSFontWeightRegular), secondary(), 1));

	NSString *temp = str(d, @"temp"), *range = str(d, @"range"), *icon = str(d, @"icon");
	CGFloat right = kW - kPad;
	if (temp.length) text(temp, right, 24, 1, attrs(font(32, NSFontWeightSemibold), primary(), 0));
	CGFloat rangeW = range.length ? text(range, right, 70, 1, attrs(font(17, NSFontWeightRegular), secondary(), 0)) : 0;
	if (icon.length) {
		NSImageSymbolConfiguration *cfg = [[NSImageSymbolConfiguration configurationWithPointSize:26 weight:NSFontWeightRegular]
		    configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[
			    [NSColor colorWithSRGBRed:0.98 green:0.78 blue:0.35 alpha:1]
		    ]]];
		NSImage *img = [[NSImage imageWithSystemSymbolName:icon accessibilityDescription:nil] imageWithSymbolConfiguration:cfg];
		if (img) {
			CGFloat iconRight = right - MAX(rangeW, 120) - 22;
			[img drawInRect:NSMakeRect(iconRight - img.size.width, 44 - img.size.height / 2, img.size.width, img.size.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
		}
	}
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
		text(pct, mr, y, 1, attrs(font(17, NSFontWeightRegular), up ? upColor() : downColor(), 0));
		text(str(m, @"price"), mr - 112, y + 1, 1, attrs(font(15, NSFontWeightRegular), secondary(), 0));
	}
	rule(478);

	// ---- Footer
	text(str(d, @"status"), kPad, 494, -1, attrs(font(16, NSFontWeightRegular), secondary(), 0));
	NSString *updated = str(d, @"updated");
	text(updated.length ? [NSString stringWithFormat:@"Prices %@ · heat.sala.company", updated] : @"Loading prices…", mr, 494,
	     1, attrs(font(16, NSFontWeightRegular), secondary(), 0));
}
@end

static NSWindow *gInfoWindow;
static WCInfoView *gInfoView;
static NSTimer *gInfoTimer;
static id gScreenObserver, gWakeObserver;

static void placeInfoWindow(void) {
	NSScreen *screen = NSScreen.screens.firstObject; // the display with the menu bar
	if (!screen || !gInfoWindow) return;
	NSRect f = screen.frame;
	CGFloat w = MIN(kW, round(f.size.width * kScreenShare)), h = round(w * kH / kW);
	NSRect r = NSMakeRect(round(NSMidX(f) - w / 2), round(NSMidY(f) - h / 2), w, h);
	[gInfoWindow setFrame:r display:YES];
	[gInfoView setBoundsSize:NSMakeSize(kW, kH)];
	gInfoView.needsDisplay = YES;
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
			return;
		}
		if (!gInfoWindow) makeInfoWindow();
		NSDictionary *d = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if ([d isKindOfClass:NSDictionary.class]) gInfoView.data = d;
		if (!gInfoWindow.visible) {
			placeInfoWindow();
			[gInfoWindow orderFront:nil];
			scheduleTick();
		}
		gInfoView.needsDisplay = YES;
	});
}
