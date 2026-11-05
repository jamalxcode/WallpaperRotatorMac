#import <Cocoa/Cocoa.h>
#include "settings_darwin.h"
#include "_cgo_export.h"

static NSString *const kShowNotification = @"com.jamalxcode.wallpaperrotator.showSettings";
static NSArray<NSString *> *weightNames(void) {
	return @[ @"Off", @"Rarely", @"Sometimes", @"Normal", @"Often", @"Very often" ];
}

// Menu-bar-only apps have no main menu, so standard shortcuts don't work by
// default. Handle the essentials here.
@interface WCWindow : NSWindow
@end
@implementation WCWindow
- (BOOL)performKeyEquivalent:(NSEvent *)e {
	NSEventModifierFlags m = e.modifierFlags & NSEventModifierFlagDeviceIndependentFlagsMask;
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

	self.thumb = [[NSImageView alloc] init];
	self.thumb.imageScaling = NSImageScaleProportionallyUpOrDown;
	self.thumb.wantsLayer = YES;
	self.thumb.layer.cornerRadius = 8;
	self.thumb.layer.masksToBounds = YES;
	self.thumb.layer.backgroundColor = NSColor.quaternaryLabelColor.CGColor;
	fixedWidth(self.thumb, inner);
	[self.thumb.heightAnchor constraintEqualToConstant:round(inner * 9 / 16)].active = YES;

	self.titleLabel = label(@"", 15, YES, NSColor.labelColor);
	self.titleLabel.preferredMaxLayoutWidth = inner;
	self.creditLabel = label(@"", 12, NO, NSColor.secondaryLabelColor);
	self.creditLabel.preferredMaxLayoutWidth = inner;

	self.prevButton = [self button:@"◀ Previous" action:@selector(previous:)];
	self.nextButton = [self button:@"Next ▶" action:@selector(next:)];
	self.nextButton.keyEquivalent = @"\r";
	self.banButton = [self button:@"Never Show Again" action:@selector(ban:)];
	self.openButton = [self button:@"Open Image Page" action:@selector(openPage:)];
	self.saveButton = [self button:@"Save to Pictures" action:@selector(save:)];

	self.intervalPopup = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
	[self.intervalPopup addItemsWithTitles:self.intervalLabels];
	self.intervalPopup.target = self;
	self.intervalPopup.action = @selector(interval:);

	self.weightSliders = [NSMutableArray array];
	self.weightLabels = [NSMutableArray array];
	NSMutableArray<NSView *> *sourceRows = [NSMutableArray array];
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
		fixedWidth(s, 120);
		NSTextField *val = label(@"", 11, NO, NSColor.secondaryLabelColor);
		fixedWidth(val, 70);
		[self.weightSliders addObject:s];
		[self.weightLabels addObject:val];
		[sourceRows addObject:row(@[ name, s, val ])];
	}

	// DeviantArt keys
	self.daIDField = [NSTextField textFieldWithString:@""];
	self.daIDField.placeholderString = @"Client ID";
	fixedWidth(self.daIDField, 110);
	self.daSecretField = [[NSSecureTextField alloc] init];
	fixedWidth(self.daSecretField, 200);
	NSButton *daConnect = [self button:@"Connect" action:@selector(daConnect:)];
	self.daStatusLabel = label(@"", 11, NO, NSColor.secondaryLabelColor);
	self.daStatusLabel.preferredMaxLayoutWidth = inner;
	self.daSkipAIBox = [self checkbox:@"Skip AI-generated DeviantArt art" action:@selector(skipAI:)];
	NSTextField *daHelp = label(@"Art on DeviantArt belongs to its artists (not public domain). Get free keys at "
	                            @"deviantart.com/developers → Register Application (Client type: Confidential).",
	                            11, NO, NSColor.tertiaryLabelColor);
	daHelp.preferredMaxLayoutWidth = inner;

	self.pauseBox = [self checkbox:@"Pause automatic changes" action:@selector(pause:)];
	self.keepRunningBox = [self checkbox:@"Keep changing on Low Power Mode and phone hotspots"
	                              action:@selector(keepRunning:)];
	self.loginBox = [self checkbox:@"Start Wallpaper Rotator at login" action:@selector(login:)];
	self.noteLabel = label(@"", 11, NO, NSColor.systemOrangeColor);
	self.noteLabel.preferredMaxLayoutWidth = inner;

	NSTextField *hint = label(@"Also in the menu bar (picture-frame icon). ⌘W closes this window.", 11, NO,
	                          NSColor.tertiaryLabelColor);
	NSView *spacer = [[NSView alloc] init];
	[spacer setContentHuggingPriority:1 forOrientation:NSLayoutConstraintOrientationHorizontal];
	NSButton *quit = [self button:@"Quit" action:@selector(quit:)];

	NSMutableArray<NSView *> *views = [NSMutableArray arrayWithArray:@[
		self.thumb,
		self.titleLabel,
		self.creditLabel,
		row(@[ self.prevButton, self.nextButton, self.banButton ]),
		row(@[ self.openButton, self.saveButton ]),
		separator(),
		row(@[ label(@"Change every", 13, NO, NSColor.labelColor), self.intervalPopup ]),
		label(@"Picture sources: how often each appears", 13, YES, NSColor.labelColor),
	]];
	[views addObjectsFromArray:sourceRows];
	[views addObjectsFromArray:@[
		separator(),
		label(@"DeviantArt", 13, YES, NSColor.labelColor),
		daHelp,
		row(@[ self.daIDField, self.daSecretField, daConnect ]),
		self.daStatusLabel,
		self.daSkipAIBox,
		separator(),
		self.pauseBox,
		self.keepRunningBox,
		self.loginBox,
		self.noteLabel,
		separator(),
		row(@[ hint, spacer, quit ]),
	]];

	NSStackView *stack = [NSStackView stackViewWithViews:views];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.alignment = NSLayoutAttributeLeading;
	stack.spacing = 8;
	stack.edgeInsets = NSEdgeInsetsMake(pad, pad, pad, pad);
	[stack setCustomSpacing:12 afterView:self.thumb];
	[stack setCustomSpacing:2 afterView:self.titleLabel];
	[stack setCustomSpacing:12 afterView:self.creditLabel];
	stack.translatesAutoresizingMaskIntoConstraints = NO;
	for (NSView *v in views) {
		if ([v isKindOfClass:[NSBox class]] || v == views.lastObject) {
			[v.widthAnchor constraintEqualToAnchor:stack.widthAnchor constant:-2 * pad].active = YES;
		}
	}
	[stack.widthAnchor constraintEqualToConstant:width].active = YES;

	self.window = [[WCWindow alloc] initWithContentRect:NSMakeRect(0, 0, width, 600)
	                                          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                                                    NSWindowStyleMaskMiniaturizable
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	self.window.title = @"Wallpaper Rotator";
	self.window.releasedWhenClosed = NO;
	self.window.delegate = self;
	self.window.contentView = stack;
	[self apply];
	[self.window center];
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
	self.daSecretField.placeholderString = self.daHasSecret ? @"Saved in Keychain" : @"Client secret";
	self.daStatusLabel.stringValue = self.daStatus ?: @"";
	self.daSkipAIBox.state = self.daSkipAI ? NSControlStateValueOn : NSControlStateValueOff;
	// keep the window snug as text wraps or notes appear
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
	[self.window orderFrontRegardless]; // menu-bar apps aren't always allowed to come forward
}

- (void)next:(id)s { goSettingsAction(WCActionNext, 0, 0); }
- (void)previous:(id)s { goSettingsAction(WCActionPrevious, 0, 0); }
- (void)ban:(id)s { goSettingsAction(WCActionBan, 0, 0); }
- (void)openPage:(id)s { goSettingsAction(WCActionOpenPage, 0, 0); }
- (void)save:(id)s { goSettingsAction(WCActionSave, 0, 0); }
- (void)quit:(id)s { goSettingsAction(WCActionQuit, 0, 0); }
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
	self.daSecretField.stringValue = @""; // the secret goes to the Keychain, not the window
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
	if (gController) [gController show];
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

void wcInit(const char *sourceLabels, const char *intervalLabels) {
	NSArray *src = [str(sourceLabels) componentsSeparatedByString:@"\n"];
	NSArray *iv = [str(intervalLabels) componentsSeparatedByString:@"\n"];
	onMain(^{
		if (gController) return;
		gController = [[WCController alloc] init];
		gController.sourceLabels = src;
		gController.intervalLabels = iv;
		gController.intervalIndex = -1;
		if (gShowWhenReady) dispatch_async(dispatch_get_main_queue(), ^{ [gController show]; });
	});
}

void wcShowSettings(void) {
	onMain(^{ [gController show]; });
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
