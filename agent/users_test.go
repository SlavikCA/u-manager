package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Entries as `getent passwd` returns them: local accounts first, then directory
// accounts, which Authentik numbers from 2000 up.
var (
	localJane = "jane:x:1000:1000:Jane:/home/jane:/bin/bash"
	dirDana   = "dana:*:2001:2001:Dana:/home/dana:/bin/bash"
	dirJane   = "jane:*:2002:2002:Jane:/home/jane:/bin/bash"
	systemd   = "systemd-resolve:x:991:991::/:/usr/sbin/nologin"
)

func TestBuildUsersMergesSources(t *testing.T) {
	danaLogin := time.Date(2026, 8, 30, 1, 2, 0, 0, time.UTC)

	users := buildUsers(
		[]string{localJane, dirDana, dirJane, systemd},
		map[string]int{"jane": 1000},
		map[string]bool{"dana": true, "ghost": true},
		map[string]time.Time{"dana": danaLogin},
	)

	got := make(map[string]UserInfo, len(users))
	for _, user := range users {
		got[user.Username] = user
	}

	if len(users) != 2 {
		t.Fatalf("want jane and dana, got %d entries: %+v", len(users), users)
	}

	// A name in both /etc/passwd and the directory keeps the local account.
	jane := got["jane"]
	if !jane.IsLocal || jane.UID != 1000 {
		t.Fatalf("want the local jane (uid 1000), got %+v", jane)
	}

	// Directory accounts are recognized as such and locked while denied.
	dana := got["dana"]
	if dana.IsLocal {
		t.Fatalf("dana is a directory account: %+v", dana)
	}
	if !dana.IsLocked || !dana.IsLoggedIn {
		t.Fatalf("denied dana should be reported as locked and logged in: %+v", dana)
	}
	// The login time reported by `who` survives for directory accounts too.
	if dana.LoggedInAt != danaLogin.Unix() {
		t.Fatalf("dana logged_in_at = %d, want %d", dana.LoggedInAt, danaLogin.Unix())
	}

	// Accounts below the UID range the app manages are ignored.
	if _, ok := got["systemd-resolve"]; ok {
		t.Fatal("system account with uid 991 should be skipped")
	}
}

// A denied account that NSS does not enumerate is looked up one by one: a
// directory account keeps its row on the server, a local one is not governed by
// the sssd deny list, and an unresolvable name is dropped.
func TestGetUsersResolvesDeniedAccountsMissingFromEnumeration(t *testing.T) {
	dir := t.TempDir()
	sssdConfDir = dir
	t.Cleanup(func() {
		sssdConfDir = "/etc/sssd/conf.d"
		lookupNSSUser = nssUser
	})

	snippet := "[domain/authentik]\nsimple_deny_users = ghost, local, gone\n"
	if err := os.WriteFile(filepath.Join(dir, denySnippetName), []byte(snippet), 0600); err != nil {
		t.Fatal(err)
	}

	lookupNSSUser = func(username string) (UserInfo, error) {
		switch username {
		case "ghost":
			return UserInfo{Username: "ghost", UID: 2004, HomeDir: "/home/ghost", Shell: "/bin/sh"}, nil
		case "local":
			return UserInfo{Username: "local", UID: 1004, IsLocal: true}, nil
		default:
			return UserInfo{}, errors.New("not found in NSS")
		}
	}

	users := getUsers()

	// The host running the tests has its own accounts, only the stubbed names
	// above are interesting here.
	for _, user := range users {
		if user.Username == "local" || user.Username == "gone" {
			t.Fatalf("%s should not be reported: %+v", user.Username, user)
		}
	}

	ghost, ok := findUser(users, "ghost")
	if !ok {
		t.Fatal("the denied directory account should still be reported")
	}
	if !ghost.IsLocked || ghost.UID != 2004 || ghost.IsLocal {
		t.Fatalf("unexpected fallback entry: %+v", ghost)
	}
}

func findUser(users []UserInfo, username string) (UserInfo, bool) {
	for _, user := range users {
		if user.Username == username {
			return user, true
		}
	}
	return UserInfo{}, false
}
