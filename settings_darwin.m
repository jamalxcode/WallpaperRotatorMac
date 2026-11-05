#import <Cocoa/Cocoa.h>
#include "settings_darwin.h"
#include "_cgo_export.h"

static NSString *const kShowNotification = @"com.yoda.wallpaperchanger.showSettings";

@interface WCController : NSObject <NSWindowDelegate>
@property(strong) NSArray<NSString *> *sourceLabels;
@property(strong) NSArray<NSString *> *intervalLabels;
@property(strong) NSWindow *window;
@property(strong) NSTextField *titleLabel, *creditLabel;
@property(strong) NSButton *nextButton, *openButton, *saveButton, *pauseBox, *loginBox;
@property(strong) NSPopUpButton *intervalPopup;
@property(strong) NSMutableArray<NSButton *> *sourceBoxes;
// last known state, applied when the window is (re)built
@property NSString *statusTitle, *statusCredit;
@property int hasImage, busy, intervalIndex, sourcesMask, paused, login;
@end

static WCController *gController;

static NSTextField *label(NSString *s, CGFloat size, BOOL bold, NSColor *color) {
	NSTextField *t = [NSTextField wrappingLabelWithString:s ?: @""];
	t.font = bold ? [NSFont boldSystemFontOfSize:size] : [NSFont systemFontOfSize:size];
	t.textColor = color;
	t.selectable = NO;
	return t;
}

static NSView *separator(void) {
	NSBox *b = [[NSBox alloc] init];
	b.boxType = NSBoxSeparator;
	return b;
}

static NSStackView *row(NSArray<NSView *> *views) {
	NSStackView *s = [NSStackView stackViewWithViews:views];
	s.orientation = NSUserInterfaceLayoutOrientationHorizontal;
	s.spacing = 8;
	return s;
}

@implementation WCController

- (NSButton *)button:(NSString *)title action:(SEL)sel {
	NSButton *b = [NSButton buttonWithTitle:title target:self action:sel];
	b.bezelStyle = NSBezelStyleRounded;
	return b;
}

- (NSButton *)checkbox:(NSString *)title action:(SEL)sel tag:(NSInteger)tag {
	NSButton *b = [NSButton checkboxWithTitle:title target:self action:sel];
	b.tag = tag;
	return b;
}

- (void)buildWindow {
	const CGFloat width = 460, pad = 20;

	self.titleLabel = label(@"", 15, YES, NSColor.labelColor);
	self.titleLabel.preferredMaxLayoutWidth = width - 2 * pad;
	self.creditLabel = label(@"", 12, NO, NSColor.secondaryLabelColor);
	self.creditLabel.preferredMaxLayoutWidth = width - 2 * pad;

	self.nextButton = [self button:@"Next Wallpaper" action:@selector(next:)];
	self.nextButton.keyEquivalent = @"\r";
	self.openButton = [self button:@"Open Image Page" action:@selector(openPage:)];
	self.saveButton = [self button:@"Save to Pictures" action:@selector(save:)];

	self.intervalPopup = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
	[self.intervalPopup addItemsWithTitles:self.intervalLabels];
	self.intervalPopup.target = self;
	self.intervalPopup.action = @selector(interval:);

	self.sourceBoxes = [NSMutableArray array];
	NSMutableArray<NSView *> *sourceViews = [NSMutableArray array];
	for (NSUInteger i = 0; i < self.sourceLabels.count; i++) {
		NSButton *b = [self checkbox:self.sourceLabels[i] action:@selector(source:) tag:(NSInteger)i];
		[self.sourceBoxes addObject:b];
		[sourceViews addObject:b];
	}

	self.pauseBox = [self checkbox:@"Pause automatic changes" action:@selector(pause:) tag:0];
	self.loginBox = [self checkbox:@"Start Wallpaper Changer at login" action:@selector(login:) tag:0];

	NSTextField *hint = label(@"Also available from the picture-frame icon in the menu bar.", 11, NO,
	                          NSColor.tertiaryLabelColor);
	NSView *spacer = [[NSView alloc] init];
	[spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
	NSButton *quit = [self button:@"Quit" action:@selector(quit:)];

	NSMutableArray<NSView *> *views = [NSMutableArray arrayWithArray:@[
		label(@"NOW SHOWING", 11, YES, NSColor.secondaryLabelColor),
		self.titleLabel,
		self.creditLabel,
		row(@[ self.nextButton, self.openButton, self.saveButton ]),
		separator(),
		row(@[ label(@"Change every", 13, NO, NSColor.labelColor), self.intervalPopup ]),
		label(@"Picture sources", 13, YES, NSColor.labelColor),
	]];
	[views addObjectsFromArray:sourceViews];
	[views addObjectsFromArray:@[
		separator(),
		self.pauseBox,
		self.loginBox,
		separator(),
		row(@[ hint, spacer, quit ]),
	]];

	NSStackView *stack = [NSStackView stackViewWithViews:views];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.alignment = NSLayoutAttributeLeading;
	stack.spacing = 10;
	stack.edgeInsets = NSEdgeInsetsMake(pad, pad, pad, pad);
	[stack setCustomSpacing:2 afterView:self.titleLabel];
	[stack setCustomSpacing:14 afterView:self.creditLabel];
	stack.translatesAutoresizingMaskIntoConstraints = NO;
	for (NSView *v in views) {
		if ([v isKindOfClass:[NSBox class]] || v == views.lastObject) {
			[v.widthAnchor constraintEqualToAnchor:stack.widthAnchor constant:-2 * pad].active = YES;
		}
	}
	[stack.widthAnchor constraintEqualToConstant:width].active = YES;

	self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, width, 400)
	                                          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                                                    NSWindowStyleMaskMiniaturizable
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	self.window.title = @"Wallpaper Changer";
	self.window.releasedWhenClosed = NO;
	self.window.delegate = self;
	self.window.contentView = stack;
	[self.window setContentSize:stack.fittingSize];
	[self.window center];
	[self apply];
}

// Push the last known state into the controls.
- (void)apply {
	if (!self.window) return;
	self.titleLabel.stringValue = self.statusTitle ?: @"Starting…";
	self.creditLabel.stringValue = self.statusCredit ?: @"";
	self.nextButton.enabled = !self.busy;
	self.openButton.enabled = self.hasImage;
	self.saveButton.enabled = self.hasImage;
	if (self.intervalIndex >= 0 && self.intervalIndex < (int)self.intervalLabels.count)
		[self.intervalPopup selectItemAtIndex:self.intervalIndex];
	for (NSButton *b in self.sourceBoxes)
		b.state = (self.sourcesMask & (1 << b.tag)) ? NSControlStateValueOn : NSControlStateValueOff;
	self.pauseBox.state = self.paused ? NSControlStateValueOn : NSControlStateValueOff;
	self.loginBox.state = self.login ? NSControlStateValueOn : NSControlStateValueOff;
	// keep the window snug if the title wraps onto more lines
	[self.window setContentSize:self.window.contentView.fittingSize];
}

- (void)show {
	if (!self.window) [self buildWindow];
	if (@available(macOS 14.0, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	[self.window makeKeyAndOrderFront:nil];
}

- (void)next:(id)s { goSettingsAction(WCActionNext, 0, 0); }
- (void)openPage:(id)s { goSettingsAction(WCActionOpenPage, 0, 0); }
- (void)save:(id)s { goSettingsAction(WCActionSave, 0, 0); }
- (void)quit:(id)s { goSettingsAction(WCActionQuit, 0, 0); }
- (void)interval:(NSPopUpButton *)p { goSettingsAction(WCActionInterval, (int)p.indexOfSelectedItem, 0); }
- (void)source:(NSButton *)b {
	goSettingsAction(WCActionSource, (int)b.tag, b.state == NSControlStateValueOn);
}
- (void)pause:(NSButton *)b { goSettingsAction(WCActionPause, 0, b.state == NSControlStateValueOn); }
- (void)login:(NSButton *)b { goSettingsAction(WCActionLogin, 0, b.state == NSControlStateValueOn); }

// Apple Event sent when the user opens the app while it is already running.
- (void)handleReopen:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	[self show];
}

- (void)showFromNotification:(NSNotification *)n {
	[self show];
}

@end

static void onMain(dispatch_block_t b) {
	if ([NSThread isMainThread]) b();
	else dispatch_async(dispatch_get_main_queue(), b);
}

void wcInit(const char *sourceLabels, const char *intervalLabels) {
	NSArray *src = [[NSString stringWithUTF8String:sourceLabels] componentsSeparatedByString:@"\n"];
	NSArray *iv = [[NSString stringWithUTF8String:intervalLabels] componentsSeparatedByString:@"\n"];
	onMain(^{
		if (gController) return;
		gController = [[WCController alloc] init];
		gController.sourceLabels = src;
		gController.intervalLabels = iv;
		gController.intervalIndex = -1;
	});
}

void wcShowSettings(void) {
	onMain(^{ [gController show]; });
}

void wcSetStatus(const char *title, const char *credit, int hasImage, int busy) {
	NSString *t = [NSString stringWithUTF8String:title];
	NSString *c = [NSString stringWithUTF8String:credit];
	onMain(^{
		gController.statusTitle = t;
		gController.statusCredit = c;
		gController.hasImage = hasImage;
		gController.busy = busy;
		[gController apply];
	});
}

void wcSetState(int intervalIndex, int sourcesMask, int paused, int login) {
	onMain(^{
		gController.intervalIndex = intervalIndex;
		gController.sourcesMask = sourcesMask;
		gController.paused = paused;
		gController.login = login;
		[gController apply];
	});
}

void wcInstallReopenHandlers(void) {
	onMain(^{
		[[NSAppleEventManager sharedAppleEventManager]
		    setEventHandler:gController
		        andSelector:@selector(handleReopen:withReplyEvent:)
		      forEventClass:kCoreEventClass
		         andEventID:kAEReopenApplication];
		[[NSDistributedNotificationCenter defaultCenter] addObserver:gController
		                                                    selector:@selector(showFromNotification:)
		                                                        name:kShowNotification
		                                                      object:nil];
	});
}

void wcPostShowToRunningInstance(void) {
	[[NSDistributedNotificationCenter defaultCenter] postNotificationName:kShowNotification
	                                                               object:nil
	                                                             userInfo:nil
	                                                   deliverImmediately:YES];
}
