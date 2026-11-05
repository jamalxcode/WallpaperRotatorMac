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
};

// Labels are newline-separated. All functions are safe to call from any thread.
void wcInit(const char *sourceLabels, const char *intervalLabels);
void wcShowSettings(void);
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
