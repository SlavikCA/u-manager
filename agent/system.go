package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type UserInfo struct {
	Username   string `json:"username"`
	UID        int    `json:"uid"`
	HomeDir    string `json:"home_dir"`
	Shell      string `json:"shell"`
	IsLocked   bool   `json:"is_locked"`
	IsLoggedIn bool   `json:"is_logged_in"`
	LoggedInAt int64  `json:"logged_in_at,omitempty"` // epoch seconds, 0 when not logged in
}

func getUsers() []UserInfo {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer file.Close()

	loggedInUsers := getLoggedInUsers()

	var users []UserInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}

		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}

		username := fields[0]
		user := UserInfo{
			Username: username,
			UID:      uid,
			HomeDir:  fields[5],
			Shell:    fields[6],
			IsLocked: isUserLocked(username),
		}
		if loginTime, ok := loggedInUsers[username]; ok {
			user.IsLoggedIn = true
			user.LoggedInAt = loginTime.Unix()
		}
		users = append(users, user)
	}

	return users
}

func isUserLocked(username string) bool {
	out, err := exec.Command("passwd", "-S", username).Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 2 {
		return fields[1] == "L"
	}
	return false
}

// getLoggedInUsers returns the earliest login time per user from `who` output.
// GNU coreutils `who` prints the login time as "YYYY-MM-DD HH:MM" for ISO
// locales and "Mon DD HH:MM" (year omitted) otherwise, so both forms are
// parsed; the year is assumed to be the current one for the short form.
// LC_ALL is pinned to C.UTF-8 for a stable format across hosts, and times
// are parsed in the machine's local timezone since `who` prints local time
// without a zone offset.
func getLoggedInUsers() map[string]time.Time {
	cmd := exec.Command("who")
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseLoggedInUsers(strings.NewReader(string(out)))
}

func parseLoggedInUsers(r io.Reader) map[string]time.Time {
	result := make(map[string]time.Time)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		// GDM greeter entry: timestamp is when the login screen appeared,
		// not when a user logged in.
		if strings.Contains(line, "(login screen)") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] == "reboot" {
			continue
		}
		t, ok := parseLoginTime(fields)
		if !ok {
			continue
		}
		if prev, ok := result[fields[0]]; !ok || t.Before(prev) {
			result[fields[0]] = t
		}
	}
	return result
}

// parseLoginTime extracts the login time from `who` fields, supporting
// "YYYY-MM-DD HH:MM" and "Mon DD HH:MM".
func parseLoginTime(fields []string) (time.Time, bool) {
	if t, err := time.ParseInLocation("2006-01-02 15:04", fields[2]+" "+fields[3], time.Local); err == nil {
		return t, true
	}
	if len(fields) >= 5 {
		// Short form omits the year. time.Parse fills in year 0, so rebuild
		// with time.Date to get the current year's zone offset.
		t, err := time.ParseInLocation("Jan 2 15:04", fields[2]+" "+fields[3]+" "+fields[4], time.Local)
		if err != nil {
			return time.Time{}, false
		}
		now := time.Now()
		t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		if t.After(now) { // Dec 31/Jan 1 boundary: who omits the year
			t = t.AddDate(-1, 0, 0)
		}
		return t, true
	}
	return time.Time{}, false
}

func getCurrentDesktopUser() string {
	sessions := getGraphicalSessions()
	if len(sessions) > 0 {
		return sessions[0].username
	}
	return ""
}

func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
