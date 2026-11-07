// Native Settings window. Implemented in settings_darwin.m; callbacks go to
// the Go functions goSettingsAction / goSettingsDeviantKeys (settings_darwin.go).
#pragma once

// Action codes passed to goSettingsAction(action, index, value).
enum {
	WCActionNext = 1,
	WCActionOpenPage = 2,
	WCActionSave = 3,
	WCActionInterval = 4,    // index = interval index
	WCActionWeight = 5,      // index = source index, value = 0..5
	WCActionPause = 6,       // value = paused
	WCActionLogin = 7,       // value = start at login
	WCActionQuit = 8,
	WCActionSkipAI = 9,      // value = skip AI-generated DeviantArt art
	WCActionPrevious = 10,
	WCActionBan = 11,        // never show the current picture again
	WCActionKeepRunning = 12, // value = keep changing on Low Power Mode / hotspot
	WCActionCheckUpdates = 13,
	WCActionPresence = 14,   // index = show in menu bar, value = show in Dock
};

// Labels are newline-separated. All functions are safe to call from any thread.
void wcInit(const char *sourceLabels, const char *intervalLabels);
// pane: 0 = current picture, -1 = the settings tab used last.
void wcShow(int pane);
// Show or hide the menu-bar icon and the Dock icon (at least one should be on).
void wcSetPresence(int menuBar, int dock);
void wcWatchLaunch(void);
// Modal alert with up to three buttons (pass NULL for unused ones); returns 1..3.
// Must not be called on the main thread.
int wcAlert(const char *title, const char *message, const char *b1, const char *b2, const char *b3);
// CFBundleShortVersionString and the bundle path; the caller frees the strings.
char *wcAppVersion(void);
char *wcBundlePath(void);
void wcSetStatus(const char *title, const char *credit, const char *imagePath, int hasImage, int busy,
                 int canGoBack);
// weights: comma-separated 0..5 per source. login: 0 off, 1 on, 2 needs approval, 3 unavailable.
void wcSetState(int intervalIndex, const char *weights, int paused, int login, int keepRunning,
                const char *note);
void wcSetDeviantArt(const char *clientID, int hasSecret, int skipAI, const char *status);
void wcSetDeviantStatus(const char *status);
// Show the window when the app is opened again (Finder, Launchpad, Spotlight).
void wcInstallReopenHandlers(void);
// Used by a second copy of the app to ask the running one to show its window.
void wcPostShowToRunningInstance(void);
