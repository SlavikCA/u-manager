package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func localTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local)
	if err != nil {
		t.Fatalf("bad expected time %q: %v", value, err)
	}
	return parsed
}

func TestParseLoggedInUsers(t *testing.T) {
	now := time.Now()
	// Mix of ISO ("YYYY-MM-DD HH:MM") and short ("Mon DD HH:MM") formats,
	// a GDM "(login screen)" greeter entry, and malformed lines.
	input := "slavik   seat0        Aug 30 00:05 (login screen)\n" +
		"slavik   :1           2026-08-30 00:05 (:1)\n" +
		"slavik   pts/0       2026-08-30 01:02   192.168.1.10\n" +
		"jane     pts/1       Aug 29 23:15   10.0.0.5\n" +
		"bob      pts/2       Aug  5 10:05\n" +
		"reboot   system boot 2026-08-29 06:12\n" +
		"malformed\n" +
		"user2    pts/3       2026-08-30\n"

	got := parseLoggedInUsers(strings.NewReader(input))

	if len(got) != 3 {
		t.Fatalf("expected 3 users, got %d: %v", len(got), got)
	}

	// Earliest real session wins; the "(login screen)" entry must not count.
	if !got["slavik"].Equal(localTime(t, "2026-08-30 00:05")) {
		t.Errorf("slavik = %v, want 2026-08-30 00:05", got["slavik"])
	}
	// Short form: year is assumed to be the current one.
	wantJane := time.Date(now.Year(), time.August, 29, 23, 15, 0, 0, time.Local)
	if !got["jane"].Equal(wantJane) {
		t.Errorf("jane = %v, want %v", got["jane"], wantJane)
	}
	// Single-digit day (space-padded in `who` output).
	wantBob := time.Date(now.Year(), time.August, 5, 10, 5, 0, 0, time.Local)
	if !got["bob"].Equal(wantBob) {
		t.Errorf("bob = %v, want %v", got["bob"], wantBob)
	}
	if _, ok := got["reboot"]; ok {
		t.Error("reboot line should be skipped")
	}
	if _, ok := got["user2"]; ok {
		t.Error("line without a parseable date/time should be skipped")
	}
}
func TestParseLoggedInUsersEmpty(t *testing.T) {
	if got := parseLoggedInUsers(strings.NewReader("")); len(got) != 0 {
		t.Fatalf("expected no users, got %v", got)
	}
}

// TestGetLoggedInUsersLive exercises the real `who` binary and locale
// pinning; skips where `who` is unavailable.
func TestGetLoggedInUsersLive(t *testing.T) {
	if _, err := exec.LookPath("who"); err != nil {
		t.Skip("who not available")
	}
	for user, login := range getLoggedInUsers() {
		d := time.Since(login)
		if d < -time.Hour || d > 400*24*time.Hour {
			t.Errorf("user %s login time %v is implausible", user, login)
		}
	}
}
