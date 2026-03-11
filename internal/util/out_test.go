package util

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogRotationDoesNotDeadlock(t *testing.T) {
	t.Parallel()

	out := &Out{
		listeners: make([]OutListener, 0),
		defOut:    io.Discard,
		defErr:    io.Discard,
	}

	logDir := t.TempDir()
	if err := out.StartLoggers(logDir); err != nil {
		t.Fatalf("StartLoggers() error = %v", err)
	}
	t.Cleanup(func() {
		if out.logoutFile != nil {
			_ = out.logoutFile.Close()
		}
		if out.logerrFile != nil {
			_ = out.logerrFile.Close()
		}
	})

	out.logoutCount = 100000

	done := make(chan struct{})
	go func() {
		out.log(INFO, "info", "trigger rotation")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("log rotation deadlocked")
	}

	if _, err := os.Stat(filepath.Join(logDir, "log_out.old")); err != nil {
		t.Fatalf("expected rotated output log, stat error = %v", err)
	}
}

func TestDisableLoggingDoesNotDeadlock(t *testing.T) {
	t.Parallel()

	out := &Out{
		listeners: make([]OutListener, 0),
		defOut:    io.Discard,
		defErr:    io.Discard,
	}

	if err := out.StartLoggers(t.TempDir()); err != nil {
		t.Fatalf("StartLoggers() error = %v", err)
	}
	t.Cleanup(func() {
		if out.logoutFile != nil {
			_ = out.logoutFile.Close()
		}
		if out.logerrFile != nil {
			_ = out.logerrFile.Close()
		}
	})

	done := make(chan struct{})
	go func() {
		out.DisableLogging()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DisableLogging deadlocked")
	}
}
