package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Keep Screen On runs macOS's caffeinate so neither the display nor the Mac
// goes to sleep. -w ties it to this app, so it also stops if the app quits.
var awake struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func setKeepAwake(on bool) {
	awake.mu.Lock()
	defer awake.mu.Unlock()
	if on == (awake.cmd != nil) {
		return
	}
	if !on {
		_ = awake.cmd.Process.Kill()
		awake.cmd = nil
		log.Print("keep screen on: off")
		return
	}
	cmd := exec.Command("/usr/bin/caffeinate", "-d", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		log.Print("keep screen on: ", err)
		return
	}
	go func() { _ = cmd.Wait() }()
	awake.cmd = cmd
	log.Print("keep screen on: on")
}
