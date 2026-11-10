package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Every way the app stops is written to the log, so "it stopped running" can
// be traced later. Force Quit and crashes can't be logged as they happen; the
// next start notices the leftover "running" marker instead.

var (
	runningMarker = filepath.Join(supportDir, "running")
	quitMu        sync.Mutex
	quitReason    string
	quitLogged    bool
)

// setQuitReason records why the app is about to quit (the first reason wins).
func setQuitReason(r string) {
	quitMu.Lock()
	defer quitMu.Unlock()
	if quitReason == "" {
		quitReason = r
	}
}

// logQuit writes the final log line and removes the running marker.
// detail is what macOS told us (who asked the app to quit), if anything.
func logQuit(detail string) {
	quitMu.Lock()
	r, done := quitReason, quitLogged
	quitLogged = true
	quitMu.Unlock()
	if done {
		return
	}
	switch {
	case r != "" && detail != "":
		log.Printf("quitting: %s (%s)", r, detail)
	case r != "":
		log.Printf("quitting: %s", r)
	case detail != "":
		log.Printf("quitting: %s", detail)
	default:
		log.Print("quitting")
	}
	_ = os.Remove(runningMarker)
}

// markRunning notes an unclean end of the previous run and leaves a marker
// that a normal quit removes.
func markRunning() {
	if b, err := os.ReadFile(runningMarker); err == nil {
		log.Printf("the previous run (started %s) ended without quitting normally: "+
			"Force Quit, a crash, or the Mac lost power", string(b))
	}
	_ = os.WriteFile(runningMarker, []byte(time.Now().Format("2006-01-02 15:04:05")), 0o644)
}

// watchSignals logs termination signals (e.g. `kill`, or macOS stopping the
// app) and quits cleanly.
func watchSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-ch
		setQuitReason("received " + signalName(s))
		quitApp()
	}()
}

func signalName(s os.Signal) string {
	switch s {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGHUP:
		return "SIGHUP"
	}
	return s.String()
}
