package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// The DeviantArt client secret is kept in a file only this user can read
// (mode 0600) in the app's support folder. It used to live in the Keychain,
// but the Keychain asks for the login password again whenever the app's
// signature changes (every update), so it moved here.
var daSecretPath = filepath.Join(supportDir, "deviantart-secret")

func readSecretFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeSecretFile(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, path)
}

// loadDeviantSecret reads the saved secret. If there is none yet but an older
// version left it in the Keychain, it is copied out once (this may show one
// last Keychain prompt) and the Keychain item is removed.
func loadDeviantSecret() string {
	if s := readSecretFile(daSecretPath); s != "" {
		return s
	}
	s := keychainGet(daKeychainAccount)
	if s == "" {
		return ""
	}
	if err := writeSecretFile(daSecretPath, s); err != nil {
		log.Print("saving DeviantArt secret: ", err)
		return s
	}
	keychainDelete(daKeychainAccount)
	log.Print("moved the DeviantArt secret from the Keychain to ", daSecretPath)
	return s
}

func saveDeviantSecret(secret string) error {
	return writeSecretFile(daSecretPath, secret)
}

func deleteDeviantSecret() {
	_ = os.Remove(daSecretPath)
	keychainDelete(daKeychainAccount)
}
