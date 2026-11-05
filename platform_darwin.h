// macOS services used by the app. Implemented in platform_darwin.m.
#pragma once

// Wallpaper: set on every screen of the current Space; re-applied when the
// user switches Space or displays change. Returns NULL or a malloc'd error.
char *wpSetWallpaper(const char *path);
void wpStartSpaceWatcher(void);

// System state: calls goSystemState(locked, screensAsleep, lowPower,
// expensiveNetwork, online) whenever any of them changes (and once at start).
void wpStartSystemMonitor(void);

// Login item via SMAppService (macOS 13+).
// Status: 0 not registered, 1 enabled, 2 needs approval in System Settings, 3 not found.
int wpLoginStatus(void);
char *wpLoginSet(int on); // NULL or malloc'd error
void wpOpenLoginItemsSettings(void);

// Keychain (generic password, service = bundle ID). Get returns malloc'd string or NULL.
int wpKeychainSet(const char *account, const char *secret);
char *wpKeychainGet(const char *account);
void wpKeychainDelete(const char *account);

// On-device image check with Apple Vision. Returns 1 on success.
int wpVisionCheck(const char *path, double *humanArea, double *swimsuit, double *skull);
