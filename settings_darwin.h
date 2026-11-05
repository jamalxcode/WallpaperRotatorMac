// Native Settings window. Implemented in settings_darwin.m; callbacks go to
// the Go function goSettingsAction (settings_darwin.go).
#pragma once

// Action codes passed to goSettingsAction(action, index, on).
enum {
	WCActionNext = 1,
	WCActionOpenPage = 2,
	WCActionSave = 3,
	WCActionInterval = 4, // index = interval index
	WCActionSource = 5,   // index = source index, on = checked
	WCActionPause = 6,    // on = paused
	WCActionLogin = 7,    // on = start at login
	WCActionQuit = 8,
};

// Labels are newline-separated. Safe to call from any thread.
void wcInit(const char *sourceLabels, const char *intervalLabels);
void wcShowSettings(void);
void wcSetStatus(const char *title, const char *credit, int hasImage, int busy);
void wcSetState(int intervalIndex, int sourcesMask, int paused, int login);
// Show the window when the app is opened again (Finder, Launchpad, Spotlight).
void wcInstallReopenHandlers(void);
// Used by a second copy of the app to ask the running one to show its window.
void wcPostShowToRunningInstance(void);
