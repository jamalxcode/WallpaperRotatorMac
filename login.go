package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var launchAgentPath = filepath.Join(must(os.UserHomeDir()), "Library", "LaunchAgents", bundleID+".plist")

func loginEnabled() bool {
	_, err := os.Stat(launchAgentPath)
	return err == nil
}

// setLogin writes or removes a LaunchAgent that starts the app at login.
// It is not loaded now (that would start a second copy); it applies from next login.
func setLogin(on bool) error {
	if !on {
		err := os.Remove(launchAgentPath)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(exe)
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key><array><string>%s</string></array>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Interactive</string>
	<key>LimitLoadToSessionType</key><string>Aqua</string>
</dict>
</plist>
`, bundleID, esc)
	_ = os.MkdirAll(filepath.Dir(launchAgentPath), 0o755)
	return os.WriteFile(launchAgentPath, []byte(plist), 0o644)
}
