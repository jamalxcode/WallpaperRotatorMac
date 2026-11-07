#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "settings_darwin.h"
#include "_cgo_export.h"

static NSString *const kShowNotification = @"com.jamalxcode.wallpaperrotator.showSettings";
static NSArray<NSString *> *weightNames(void) {
	return @[ @"Off", @"Rarely", @"Sometimes", @"Normal", @"Often", @"Very often" ];
}

// Without a Dock icon the app's main menu isn't on screen, so make sure the
// standard shortcuts still work in the window.
@interface WCWindow : NSWindow
@end
@implementation WCWindow
- (BOOL)performKeyEquivalent:(NSEvent *)e {
	NSEventModifierFlags m = e.modifierFlags & NSEventModifierFlagDeviceIndependentFlagsMask;
	if (m == (NSEventModifierFlagCommand | NSEventModifierFlagShift) &&
	    [e.charactersIgnoringModifiers.lowercaseString isEqualToString:@"z"] &&
	    [NSApp sendAction:@selector(redo:) to:nil from:self]) {
		return YES;
	}
	if (m == NSEventModifierFlagCommand) {
		NSString *k = e.charactersIgnoringModifiers.lowercaseString;
		SEL sel = NULL;
		if ([k isEqualToString:@"w"]) { [self performClose:nil]; return YES; }
		if ([k isEqualToString:@"q"]) { goSettingsAction(WCActionQuit, 0, 0); return YES; }
		if ([k isEqualToString:@"x"]) sel = @selector(cut:);
		if ([k isEqualToString:@"c"]) sel = @selector(copy:);
		if ([k isEqualToString:@"v"]) sel = @selector(paste:);
		if ([k isEqualToString:@"a"]) sel = @selector(selectAll:);
		if ([k isEqualToString:@"z"]) sel = @selector(undo:);
		if ([k isEqualToString:@","]) sel = @selector(openSettings:); // reaches the window's delegate
		if (sel && [NSApp sendAction:sel to:nil from:self]) return YES;
	}
	return [super performKeyEquivalent:e];
}
@end

@interface WCController : NSObject <NSWindowDelegate>
@property(strong) NSArray<NSString *> *sourceLabels, *intervalLabels;
@property(strong) WCWindow *window;
@property(strong) NSImageView *thumb;
@property(strong) NSTextField *titleLabel, *creditLabel, *noteLabel;
@property(strong) NSButton *prevButton, *nextButton, *banButton, *openButton, *saveButton;
@property(strong) NSButton *pauseBox, *keepRunningBox, *loginBox;
@property(strong) NSPopUpButton *intervalPopup;
@property(strong) NSMutableArray<NSSlider *> *weightSliders;
@property(strong) NSMutableArray<NSTextField *> *weightLabels;
@property(strong) NSTextField *daIDField, *daStatusLabel;
@property(strong) NSSecureTextField *daSecretField;
@property(strong) NSButton *daSkipAIBox;
@property(strong) NSButton *menuBarBox, *dockBox;
@property(strong) NSSegmentedControl *paneControl;
@property(strong) NSArray<NSStackView *> *panes;
@property NSInteger lastSettingsPane;
@property int showMenuBar, showDock;
// last known state, applied whenever the window is (re)built or updated
@property(copy) NSString *statusTitle, *statusCredit, *imagePath, *shownImagePath, *note;
@property(copy) NSArray<NSNumber *> *weights;
@property int hasImage, busy, canGoBack, intervalIndex, paused, login, keepRunning;
@property(copy) NSString *daID, *daStatus;
@property int daHasSecret, daSkipAI;
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

static void fixedWidth(NSView *v, CGFloat w) { [v.widthAnchor constraintEqualToConstant:w].active = YES; }

static NSStackView *pane(NSArray<NSView *> *views) {
	NSStackView *s = [NSStackView stackViewWithViews:views];
	s.orientation = NSUserInterfaceLayoutOrientationVertical;
	s.alignment = NSLayoutAttributeLeading;
	s.spacing = 10;
	s.edgeInsets = NSEdgeInsetsMake(0, 20, 0, 20);
	return s;
}

@implementation WCController

- (NSButton *)button:(NSString *)title action:(SEL)sel {
	NSButton *b = [NSButton buttonWithTitle:title target:self action:sel];
	b.bezelStyle = NSBezelStyleRounded;
	return b;
}

- (NSButton *)checkbox:(NSString *)title action:(SEL)sel {
	return [NSButton checkboxWithTitle:title target:self action:sel];
}

- (void)buildWindow {
	const CGFloat width = 480, pad = 20, inner = width - 2 * pad;

	// ---- Now Showing
	self.thumb = [[NSImageView alloc] init];
	self.thumb.imageScaling = NSImageScaleProportionallyUpOrDown;
	self.thumb.wantsLayer = YES;
	self.thumb.layer.cornerRadius = 8;
	self.thumb.layer.masksToBounds = YES;
	self.thumb.layer.backgroundColor = NSColor.quaternaryLabelColor.CGColor;
	self.thumb.accessibilityLabel = @"Current wallpaper";
	fixedWidth(self.thumb, inner);
	[self.thumb.heightAnchor constraintEqualToConstant:round(inner * 9 / 16)].active = YES;

	self.titleLabel = label(@"", 15, YES, NSColor.labelColor);
	self.titleLabel.preferredMaxLayoutWidth = inner;
	self.creditLabel = label(@"", 12, NO, NSColor.secondaryLabelColor);
	self.creditLabel.preferredMaxLayoutWidth = inner;

	self.prevButton = [self button:@"◀ Previous" action:@selector(previous:)];
	self.prevButton.accessibilityLabel = @"Previous Wallpaper";
	self.nextButton = [self button:@"Next ▶" action:@selector(next:)];
	self.nextButton.accessibilityLabel = @"Next Wallpaper";
	self.nextButton.keyEquivalent = @"\r";
	self.banButton = [self button:@"Never Show This Again" action:@selector(ban:)];
	self.openButton = [self button:@"Open Image Page" action:@selector(openPage:)];
	self.saveButton = [self button:@"Save to Pictures" action:@selector(save:)];
	self.pauseBox = [self checkbox:@"Pause automatic changes" action:@selector(pause:)];
	self.noteLabel = label(@"", 11, NO, NSColor.systemOrangeColor);
	self.noteLabel.preferredMaxLayoutWidth = inner;

	NSStackView *nowPane = pane(@[
		self.thumb,
		self.titleLabel,
		self.creditLabel,
		row(@[ self.prevButton, self.nextButton, self.banButton ]),
		row(@[ self.openButton, self.saveButton ]),
		self.pauseBox,
		self.noteLabel,
	]);
	[nowPane setCustomSpacing:4 afterView:self.titleLabel];
	[nowPane setCustomSpacing:12 afterView:self.creditLabel];

	// ---- Sources
	self.intervalPopup = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
	[self.intervalPopup addItemsWithTitles:self.intervalLabels];
	self.intervalPopup.target = self;
	self.intervalPopup.action = @selector(interval:);
	self.intervalPopup.accessibilityLabel = @"Change every";

	self.weightSliders = [NSMutableArray array];
	self.weightLabels = [NSMutableArray array];
	NSMutableArray<NSView *> *sourceViews = [NSMutableArray arrayWithArray:@[
		row(@[ label(@"Change every", 13, NO, NSColor.labelColor), self.intervalPopup ]),
		label(@"How often each source appears", 13, YES, NSColor.labelColor),
	]];
	for (NSUInteger i = 0; i < self.sourceLabels.count; i++) {
		NSTextField *name = label(self.sourceLabels[i], 12, NO, NSColor.labelColor);
		name.maximumNumberOfLines = 1;
		name.lineBreakMode = NSLineBreakByTruncatingTail;
		name.toolTip = self.sourceLabels[i];
		fixedWidth(name, 230);
		NSSlider *s = [NSSlider sliderWithValue:0 minValue:0 maxValue:5 target:self action:@selector(weight:)];
		s.numberOfTickMarks = 6;
		s.allowsTickMarkValuesOnly = YES;
		s.continuous = NO;
		s.tag = (NSInteger)i;
		s.accessibilityLabel = [NSString stringWithFormat:@"How often %@ appears", self.sourceLabels[i]];
		fixedWidth(s, 120);
		NSTextField *val = label(@"", 11, NO, NSColor.secondaryLabelColor);
		fixedWidth(val, 70);
		[self.weightSliders addObject:s];
		[self.weightLabels addObject:val];
		[sourceViews addObject:row(@[ name, s, val ])];
	}
	NSStackView *sourcesPane = pane(sourceViews);

	// ---- DeviantArt
	self.daIDField = [NSTextField textFieldWithString:@""];
	self.daIDField.placeholderString = @"Client ID";
	self.daIDField.accessibilityLabel = @"DeviantArt client ID";
	fixedWidth(self.daIDField, 110);
	self.daSecretField = [[NSSecureTextField alloc] init];
	self.daSecretField.accessibilityLabel = @"DeviantArt client secret";
	fixedWidth(self.daSecretField, 200);
	NSButton *daConnect = [self button:@"Connect" action:@selector(daConnect:)];
	self.daStatusLabel = label(@"", 11, NO, NSColor.secondaryLabelColor);
	self.daStatusLabel.preferredMaxLayoutWidth = inner;
	self.daSkipAIBox = [self checkbox:@"Skip AI-generated DeviantArt art" action:@selector(skipAI:)];
	NSTextField *daHelp = label(@"Art on DeviantArt belongs to its artists (not public domain). Get free keys at "
	                            @"deviantart.com/developers → Register Application (Client type: Confidential). "
	                            @"You enter them once per Mac.",
	                            11, NO, NSColor.secondaryLabelColor);
	daHelp.preferredMaxLayoutWidth = inner;
	NSStackView *daPane = pane(@[
		daHelp,
		row(@[ self.daIDField, self.daSecretField, daConnect ]),
		self.daStatusLabel,
		self.daSkipAIBox,
	]);

	// ---- General
	self.loginBox = [self checkbox:@"Start Wallpaper Rotator at login" action:@selector(login:)];
	self.keepRunningBox = [self checkbox:@"Keep changing on Low Power Mode and phone hotspots"
	                              action:@selector(keepRunning:)];
	self.menuBarBox = [self checkbox:@"Show in menu bar" action:@selector(presence:)];
	self.dockBox = [self checkbox:@"Show in Dock" action:@selector(presence:)];
	NSTextField *presenceHelp =
	    label(@"At least one stays on. Without the Dock icon the app is also left out of ⌘-Tab and Force Quit; "
	          @"open it from the menu bar or by opening the app again.",
	          11, NO, NSColor.secondaryLabelColor);
	presenceHelp.preferredMaxLayoutWidth = inner;
	NSStackView *generalPane = pane(@[
		self.loginBox,
		self.keepRunningBox,
		separator(),
		self.menuBarBox,
		self.dockBox,
		presenceHelp,
	]);

	self.panes = @[ nowPane, sourcesPane, daPane, generalPane ];
	self.paneControl = [NSSegmentedControl segmentedControlWithLabels:@[ @"Now Showing", @"Sources", @"DeviantArt", @"General" ]
	                                                     trackingMode:NSSegmentSwitchTrackingSelectOne
	                                                           target:self
	                                                           action:@selector(pane:)];
	self.paneControl.accessibilityLabel = @"Section";

	NSMutableArray<NSView *> *views = [NSMutableArray arrayWithObject:self.paneControl];
	[views addObjectsFromArray:self.panes];
	NSStackView *stack = [NSStackView stackViewWithViews:views];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.alignment = NSLayoutAttributeCenterX;
	stack.spacing = 16;
	stack.edgeInsets = NSEdgeInsetsMake(14, 0, pad, 0);
	stack.translatesAutoresizingMaskIntoConstraints = NO;
	[stack.widthAnchor constraintEqualToConstant:width].active = YES;
	for (NSStackView *p in self.panes) {
		[p.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;
		for (NSView *v in p.arrangedSubviews) {
			if ([v isKindOfClass:[NSBox class]]) {
				[v.widthAnchor constraintEqualToAnchor:p.widthAnchor constant:-2 * pad].active = YES;
			}
		}
	}

	self.window = [[WCWindow alloc] initWithContentRect:NSMakeRect(0, 0, width, 500)
	                                          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                                                    NSWindowStyleMaskMiniaturizable
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	self.window.title = @"Wallpaper Rotator";
	self.window.releasedWhenClosed = NO;
	// Always open on the desktop (Space) the user is looking at, not the one it was last on.
	self.window.collectionBehavior = NSWindowCollectionBehaviorMoveToActiveSpace;
	self.window.delegate = self;
	self.window.contentView = stack;
	[self selectPane:0];
	[self apply];
	// Reopen where the user last put it; centre the very first time.
	if (![self.window setFrameUsingName:@"MainWindow"]) [self.window center];
	self.window.frameAutosaveName = @"MainWindow";
	[self fitWindow];
}

- (void)selectPane:(NSInteger)i {
	if (i < 0 || i >= (NSInteger)self.panes.count) i = 0;
	self.paneControl.selectedSegment = i;
	for (NSInteger j = 0; j < (NSInteger)self.panes.count; j++) self.panes[j].hidden = j != i;
	if (i > 0) self.lastSettingsPane = i;
	[self fitWindow];
}

- (void)pane:(NSSegmentedControl *)c { [self selectPane:c.selectedSegment]; }

// Size the window to the visible pane, keeping its top edge in place and the
// whole window on screen.
- (void)fitWindow {
	if (!self.window) return;
	[self.window.contentView layoutSubtreeIfNeeded];
	NSSize s = self.window.contentView.fittingSize;
	NSRect old = self.window.frame;
	NSRect f = [self.window frameRectForContentRect:NSMakeRect(0, 0, s.width, s.height)];
	f.origin.x = old.origin.x;
	f.origin.y = NSMaxY(old) - f.size.height;
	NSScreen *screen = self.window.screen ?: NSScreen.mainScreen;
	if (screen) {
		NSRect vis = screen.visibleFrame;
		if (NSMaxY(f) > NSMaxY(vis)) f.origin.y = NSMaxY(vis) - f.size.height;
		if (f.origin.y < vis.origin.y) f.origin.y = vis.origin.y;
	}
	if (!NSEqualRects(f, old)) [self.window setFrame:f display:YES animate:self.window.visible];
}

// Push the last known state into the controls.
- (void)apply {
	if (!self.window) return;
	self.titleLabel.stringValue = self.statusTitle ?: @"Starting…";
	self.creditLabel.stringValue = self.statusCredit ?: @"";
	if (![self.shownImagePath isEqualToString:self.imagePath ?: @""]) {
		self.shownImagePath = self.imagePath ?: @"";
		self.thumb.image = self.imagePath.length ? [[NSImage alloc] initWithContentsOfFile:self.imagePath] : nil;
	}
	self.nextButton.enabled = !self.busy;
	self.prevButton.enabled = !self.busy && self.canGoBack;
	self.banButton.enabled = !self.busy && self.hasImage;
	self.openButton.enabled = self.hasImage;
	self.saveButton.enabled = self.hasImage;
	if (self.intervalIndex >= 0 && self.intervalIndex < (int)self.intervalLabels.count)
		[self.intervalPopup selectItemAtIndex:self.intervalIndex];
	for (NSUInteger i = 0; i < self.weightSliders.count; i++) {
		int w = i < self.weights.count ? self.weights[i].intValue : 0;
		self.weightSliders[i].integerValue = w;
		self.weightLabels[i].stringValue = weightNames()[MAX(0, MIN(5, w))];
	}
	self.pauseBox.state = self.paused ? NSControlStateValueOn : NSControlStateValueOff;
	self.keepRunningBox.state = self.keepRunning ? NSControlStateValueOn : NSControlStateValueOff;
	self.loginBox.state = self.login == 1   ? NSControlStateValueOn
	                      : self.login == 2 ? NSControlStateValueMixed
	                                        : NSControlStateValueOff;
	self.loginBox.enabled = self.login != 3;
	self.noteLabel.stringValue = self.note ?: @"";
	self.noteLabel.hidden = self.note.length == 0;
	if (self.daIDField.currentEditor == nil) self.daIDField.stringValue = self.daID ?: @"";
	self.daSecretField.placeholderString = self.daHasSecret ? @"Saved on this Mac" : @"Client secret";
	self.daStatusLabel.stringValue = self.daStatus ?: @"";
	self.daSkipAIBox.state = self.daSkipAI ? NSControlStateValueOn : NSControlStateValueOff;
	self.menuBarBox.state = self.showMenuBar ? NSControlStateValueOn : NSControlStateValueOff;
	self.dockBox.state = self.showDock ? NSControlStateValueOn : NSControlStateValueOff;
	// the box that is the only way left to reach the app can't be turned off
	self.menuBarBox.enabled = !(self.showMenuBar && !self.showDock);
	self.dockBox.enabled = !(self.showDock && !self.showMenuBar);
	self.thumb.accessibilityLabel = self.statusTitle.length
	    ? [@"Current wallpaper: " stringByAppendingString:self.statusTitle] : @"Current wallpaper";
	// keep the window snug as text wraps or notes appear
	[self fitWindow];
}

// pane: 0 shows the current picture, -1 the settings tab used last.
- (void)showPane:(NSInteger)pane {
	if (!self.window) [self buildWindow];
	[self selectPane:pane < 0 ? MAX(1, self.lastSettingsPane) : pane];
	[self show];
}

- (void)show {
	if (!self.window) [self buildWindow];
	if (@available(macOS 14.0, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	[self.window makeKeyAndOrderFront:nil];
	[self.window orderFrontRegardless]; // menu-bar apps aren't always allowed to come forward
}

- (void)next:(id)s { goSettingsAction(WCActionNext, 0, 0); }
- (void)previous:(id)s { goSettingsAction(WCActionPrevious, 0, 0); }
- (void)ban:(id)s { goSettingsAction(WCActionBan, 0, 0); }
- (void)openPage:(id)s { goSettingsAction(WCActionOpenPage, 0, 0); }
- (void)save:(id)s { goSettingsAction(WCActionSave, 0, 0); }
- (void)quit:(id)s { goSettingsAction(WCActionQuit, 0, 0); }
- (void)openSettings:(id)s { [self showPane:-1]; }
- (void)showMain:(id)s { [self showPane:0]; }
- (void)presence:(NSButton *)b {
	int menuBar = self.menuBarBox.state == NSControlStateValueOn;
	int dock = self.dockBox.state == NSControlStateValueOn;
	if (!menuBar && !dock) { // never leave the app unreachable
		if (b == self.menuBarBox) dock = 1; else menuBar = 1;
	}
	goSettingsAction(WCActionPresence, menuBar, dock);
}
- (void)help:(id)s {
	[NSWorkspace.sharedWorkspace openURL:[NSURL URLWithString:@"https://github.com/jamalxcode/WallpaperRotatorMac#readme"]];
}
- (void)checkUpdates:(id)s { goSettingsAction(WCActionCheckUpdates, 0, 0); }
- (void)about:(id)s {
	[NSApp orderFrontStandardAboutPanel:nil];
	[NSApp activateIgnoringOtherApps:YES];
}
- (void)interval:(NSPopUpButton *)p { goSettingsAction(WCActionInterval, (int)p.indexOfSelectedItem, 0); }
- (void)weight:(NSSlider *)s {
	int w = (int)s.integerValue;
	self.weightLabels[s.tag].stringValue = weightNames()[MAX(0, MIN(5, w))];
	goSettingsAction(WCActionWeight, (int)s.tag, w);
}
- (BOOL)on:(NSButton *)b { return b.state == NSControlStateValueOn; }
- (void)pause:(NSButton *)b { goSettingsAction(WCActionPause, 0, [self on:b]); }
- (void)keepRunning:(NSButton *)b { goSettingsAction(WCActionKeepRunning, 0, [self on:b]); }
- (void)login:(NSButton *)b {
	// a "mixed" (needs approval) box cycles to on; treat anything but off as on
	goSettingsAction(WCActionLogin, 0, b.state != NSControlStateValueOff);
}
- (void)skipAI:(NSButton *)b { goSettingsAction(WCActionSkipAI, 0, [self on:b]); }
- (void)daConnect:(id)s {
	[self.window makeFirstResponder:nil]; // commit any in-progress edit
	NSCharacterSet *ws = NSCharacterSet.whitespaceAndNewlineCharacterSet;
	NSString *cid = [self.daIDField.stringValue stringByTrimmingCharactersInSet:ws];
	NSString *sec = [self.daSecretField.stringValue stringByTrimmingCharactersInSet:ws];
	self.daSecretField.stringValue = @""; // the secret is saved to its own file, not kept in the window
	self.daID = cid;
	self.daStatus = @"Connecting…";
	self.daStatusLabel.stringValue = self.daStatus;
	goSettingsDeviantKeys((char *)cid.UTF8String, (char *)sec.UTF8String);
}

// Apple Event sent when the user opens the app while it is already running.
- (void)handleReopen:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	[self show];
}
- (void)showFromNotification:(NSNotification *)n { [self show]; }
// Main window of the app: zooming isn't offered because it has a fixed size.
- (BOOL)windowShouldZoom:(NSWindow *)w toFrame:(NSRect)f { return NO; }
@end

static void onMain(dispatch_block_t b) {
	if ([NSThread isMainThread]) b();
	else dispatch_async(dispatch_get_main_queue(), b);
}

static NSString *str(const char *s) { return s ? [NSString stringWithUTF8String:s] : @""; }

// ---- launch: show Settings when the user starts the app, not at login ----

static BOOL gShowWhenReady;

@interface WCLaunchHandler : NSObject
@end
@implementation WCLaunchHandler
- (void)handleOpen:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	NSAppleEventDescriptor *prop = [event paramDescriptorForKeyword:keyAEPropData];
	BOOL atLogin = prop && prop.enumCodeValue == keyAELaunchedAsLogInItem;
	goLaunchKind(atLogin ? 1 : 0);
	if (atLogin) return;
	if (gController) [gController showPane:0];
	else gShowWhenReady = YES; // the window is built once the app has finished setting up
}
@end

static WCLaunchHandler *gLaunchHandler;

// Called from main() before the event loop starts. AppKit installs its own
// launch handler while starting up, so ours goes in right after that, before
// the launch event is delivered.
void wcWatchLaunch(void) {
	gLaunchHandler = [[WCLaunchHandler alloc] init];
	[[NSNotificationCenter defaultCenter]
	    addObserverForName:NSApplicationWillFinishLaunchingNotification
	                object:nil
	                 queue:nil
	            usingBlock:^(NSNotification *n) {
		            [[NSAppleEventManager sharedAppleEventManager] setEventHandler:gLaunchHandler
		                                                               andSelector:@selector(handleOpen:withReplyEvent:)
		                                                             forEventClass:kCoreEventClass
		                                                                andEventID:kAEOpenApplication];
	            }];
}

// ---- menus: the app menu at the top of the screen and the Dock icon menu ----

static NSMenuItem *menuItem(NSString *title, SEL action, NSString *key, id target) {
	NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key ?: @""];
	it.target = target;
	return it;
}

static NSMenu *gDockMenu;

// The app delegate belongs to the systray library, so the Dock menu method is
// added to its class at runtime.
static NSMenu *dockMenuIMP(id self, SEL _cmd, NSApplication *app) { return gDockMenu; }

static void installMenus(WCController *c) {
	NSString *name = @"Wallpaper Rotator";
	NSMenu *main = [[NSMenu alloc] init];
	NSMenuItem *(^top)(NSMenu *) = ^NSMenuItem *(NSMenu *m) {
		NSMenuItem *it = [[NSMenuItem alloc] init];
		it.submenu = m;
		[main addItem:it];
		return it;
	};

	NSMenu *appMenu = [[NSMenu alloc] initWithTitle:name];
	[appMenu addItem:menuItem([@"About " stringByAppendingString:name], @selector(about:), nil, c)];
	[appMenu addItem:menuItem(@"Check for Updates…", @selector(checkUpdates:), nil, c)];
	[appMenu addItem:NSMenuItem.separatorItem];
	[appMenu addItem:menuItem(@"Settings…", @selector(openSettings:), @",", c)];
	[appMenu addItem:NSMenuItem.separatorItem];
	NSMenu *services = [[NSMenu alloc] initWithTitle:@"Services"];
	NSMenuItem *servicesItem = menuItem(@"Services", nil, nil, nil);
	servicesItem.submenu = services;
	[appMenu addItem:servicesItem];
	NSApp.servicesMenu = services;
	[appMenu addItem:NSMenuItem.separatorItem];
	[appMenu addItem:menuItem([@"Hide " stringByAppendingString:name], @selector(hide:), @"h", nil)];
	NSMenuItem *others = menuItem(@"Hide Others", @selector(hideOtherApplications:), @"h", nil);
	others.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagOption;
	[appMenu addItem:others];
	[appMenu addItem:menuItem(@"Show All", @selector(unhideAllApplications:), nil, nil)];
	[appMenu addItem:NSMenuItem.separatorItem];
	[appMenu addItem:menuItem([@"Quit " stringByAppendingString:name], @selector(quit:), @"q", c)];
	top(appMenu);

	NSMenu *file = [[NSMenu alloc] initWithTitle:@"File"];
	[file addItem:menuItem(@"Close", @selector(performClose:), @"w", nil)];
	top(file);

	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItem:menuItem(@"Undo", @selector(undo:), @"z", nil)];
	[edit addItem:menuItem(@"Redo", @selector(redo:), @"Z", nil)];
	[edit addItem:NSMenuItem.separatorItem];
	[edit addItem:menuItem(@"Cut", @selector(cut:), @"x", nil)];
	[edit addItem:menuItem(@"Copy", @selector(copy:), @"c", nil)];
	[edit addItem:menuItem(@"Paste", @selector(paste:), @"v", nil)];
	[edit addItem:menuItem(@"Select All", @selector(selectAll:), @"a", nil)];
	top(edit);

	NSMenu *win = [[NSMenu alloc] initWithTitle:@"Window"];
	[win addItem:menuItem(@"Minimize", @selector(performMiniaturize:), @"m", nil)];
	[win addItem:menuItem(@"Zoom", @selector(performZoom:), nil, nil)];
	[win addItem:NSMenuItem.separatorItem];
	[win addItem:menuItem(name, @selector(showMain:), @"0", c)];
	[win addItem:NSMenuItem.separatorItem];
	[win addItem:menuItem(@"Bring All to Front", @selector(arrangeInFront:), nil, nil)];
	top(win);
	NSApp.windowsMenu = win;

	NSMenu *help = [[NSMenu alloc] initWithTitle:@"Help"];
	[help addItem:menuItem([name stringByAppendingString:@" Help"], @selector(help:), @"?", c)];
	top(help);
	NSApp.helpMenu = help;

	NSApp.mainMenu = main;

	gDockMenu = [[NSMenu alloc] init];
	[gDockMenu addItem:menuItem(@"Next Wallpaper", @selector(next:), nil, c)];
	[gDockMenu addItem:NSMenuItem.separatorItem];
	[gDockMenu addItem:menuItem(@"Settings…", @selector(openSettings:), nil, c)];
	[gDockMenu addItem:menuItem(@"Check for Updates…", @selector(checkUpdates:), nil, c)];
	id delegate = NSApp.delegate;
	if (delegate) {
		class_addMethod([delegate class], @selector(applicationDockMenu:), (IMP)dockMenuIMP, "@@:@");
	}
}

void wcInit(const char *sourceLabels, const char *intervalLabels) {
	NSArray *src = [str(sourceLabels) componentsSeparatedByString:@"\n"];
	NSArray *iv = [str(intervalLabels) componentsSeparatedByString:@"\n"];
	onMain(^{
		if (gController) return;
		gController = [[WCController alloc] init];
		gController.sourceLabels = src;
		gController.intervalLabels = iv;
		gController.intervalIndex = -1;
		installMenus(gController);
		if (gShowWhenReady) dispatch_async(dispatch_get_main_queue(), ^{ [gController showPane:0]; });
	});
}

void wcShow(int pane) {
	onMain(^{ [gController showPane:pane]; });
}

// The menu-bar icon belongs to the systray library; reach its status item.
static NSStatusItem *statusItem(void) {
	id d = NSApp.delegate;
	if (!d) return nil;
	Ivar iv = class_getInstanceVariable([d class], "statusItem");
	return iv ? object_getIvar(d, iv) : nil;
}

void wcSetPresence(int menuBar, int dock) {
	onMain(^{
		gController.showMenuBar = menuBar;
		gController.showDock = dock;
		statusItem().visible = menuBar ? YES : NO;
		NSApplicationActivationPolicy want = dock ? NSApplicationActivationPolicyRegular
		                                          : NSApplicationActivationPolicyAccessory;
		if (NSApp.activationPolicy != want) {
			BOOL wasVisible = gController.window.visible;
			[NSApp setActivationPolicy:want];
			// switching hides the app's windows; bring the window back if it was open
			if (wasVisible) dispatch_async(dispatch_get_main_queue(), ^{ [gController show]; });
		}
		[gController apply];
	});
}

void wcSetStatus(const char *title, const char *credit, const char *imagePath, int hasImage, int busy,
                 int canGoBack) {
	NSString *t = str(title), *c = str(credit), *p = str(imagePath);
	onMain(^{
		gController.statusTitle = t;
		gController.statusCredit = c;
		gController.imagePath = p;
		gController.hasImage = hasImage;
		gController.busy = busy;
		gController.canGoBack = canGoBack;
		[gController apply];
	});
}

void wcSetState(int intervalIndex, const char *weights, int paused, int login, int keepRunning,
                const char *note) {
	NSMutableArray<NSNumber *> *ws = [NSMutableArray array];
	for (NSString *part in [str(weights) componentsSeparatedByString:@","]) {
		if (part.length) [ws addObject:@(part.intValue)];
	}
	NSString *n = str(note);
	onMain(^{
		gController.intervalIndex = intervalIndex;
		gController.weights = ws;
		gController.paused = paused;
		gController.login = login;
		gController.keepRunning = keepRunning;
		gController.note = n;
		[gController apply];
	});
}

void wcSetDeviantArt(const char *clientID, int hasSecret, int skipAI, const char *status) {
	NSString *i = str(clientID), *st = str(status);
	onMain(^{
		gController.daID = i;
		gController.daHasSecret = hasSecret;
		gController.daSkipAI = skipAI;
		gController.daStatus = st;
		[gController apply];
	});
}

void wcSetDeviantStatus(const char *status) {
	NSString *st = str(status);
	onMain(^{
		gController.daStatus = st;
		[gController apply];
	});
}

void wcInstallReopenHandlers(void) {
	onMain(^{
		[[NSAppleEventManager sharedAppleEventManager] setEventHandler:gController
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

int wcAlert(const char *title, const char *message, const char *b1, const char *b2, const char *b3) {
	NSString *t = str(title), *m = str(message);
	NSMutableArray<NSString *> *buttons = [NSMutableArray array];
	if (b1) [buttons addObject:str(b1)];
	if (b2) [buttons addObject:str(b2)];
	if (b3) [buttons addObject:str(b3)];
	__block NSModalResponse r = NSAlertFirstButtonReturn;
	dispatch_sync(dispatch_get_main_queue(), ^{
		NSAlert *a = [[NSAlert alloc] init];
		a.messageText = t;
		a.informativeText = m;
		for (NSString *b in buttons) [a addButtonWithTitle:b];
		// the last button (Later, Cancel…) also answers to the Escape key
		if (buttons.count > 1) a.buttons.lastObject.keyEquivalent = @"\033";
		[NSApp activateIgnoringOtherApps:YES];
		r = [a runModal];
	});
	return (int)(r - NSAlertFirstButtonReturn) + 1;
}

char *wcAppVersion(void) {
	NSString *v = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
	return strdup((v ?: @"").UTF8String);
}

char *wcBundlePath(void) { return strdup(NSBundle.mainBundle.bundlePath.UTF8String ?: ""); }
