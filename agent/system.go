package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// nssTimeout bounds the NSS enumeration below. getent blocks while a remote
// directory (SSSD/LDAP) is unreachable, and a stuck getent would stall the
// heartbeat loop that runs it.
const nssTimeout = 30 * time.Second

type UserInfo struct {
	Username   string `json:"username"`
	UID        int    `json:"uid"`
	HomeDir    string `json:"home_dir"`
	Shell      string `json:"shell"`
	IsLocal    bool   `json:"is_local"`
	IsLocked   bool   `json:"is_locked"`
	IsLoggedIn bool   `json:"is_logged_in"`
	LoggedInAt int64  `json:"logged_in_at,omitempty"` // epoch seconds, 0 when not logged in
}

// serviceAccountPrefix identifies the machine's own service accounts. They are
// not people, so the agent does not report them and does not act on commands
// that name them. The match is on the leading part of the name and
// case-insensitive, so service, service-backup and Service all qualify.
const serviceAccountPrefix = "service"

func isServiceAccount(username string) bool {
	return strings.HasPrefix(strings.ToLower(username), serviceAccountPrefix)
}

// getUsers lists the accounts NSS knows about, so users from remote directories
// (SSSD/LDAP, e.g. Authentik) show up next to local /etc/passwd accounts.
//
//	IsLocal   - the account lives in /etc/passwd; only those can be locked with
//	            usermod. Directory accounts are denied through the sssd deny list.
//	IsLocked  - local account: passwd -S; directory account: present in the
//	            deny list (a directory account has no local shadow entry, so
//	            passwd -S cannot answer for it).
func getUsers() []UserInfo {
	denied := deniedUsernames()
	users := buildUsers(nssPasswdEntries(), localUIDs(), denied, getLoggedInUsers())

	// A denied account can be missing from the NSS enumeration (it is filtered by
	// nsswitch, or by sssd's filter_users). Look those up one by one so they keep
	// their row on the server and can still be enabled from the app. Accounts the
	// directory no longer returns are dropped instead of being reported blindly.
	reported := make(map[string]bool, len(users))
	for _, user := range users {
		reported[user.Username] = true
	}
	for username := range denied {
		if reported[username] || isServiceAccount(username) {
			continue
		}
		user, ok := resolveDeniedUser(username)
		if !ok {
			continue
		}
		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	return users
}

// lookupNSSUser is indirected so the fallback below can be tested without
// depending on the accounts of the host running the tests.
var lookupNSSUser = nssUser

// resolveDeniedUser looks up a denied account that the NSS enumeration did not
// return. Only directory accounts qualify: the sssd deny list does not apply to
// local ones, and a name NSS cannot resolve at all (the directory dropped it) is
// not reported instead of being reported blindly.
func resolveDeniedUser(username string) (UserInfo, bool) {
	user, err := lookupNSSUser(username)
	if err != nil {
		log.Printf("Denied account %s is not returned by NSS: %v", username, err)
		return UserInfo{}, false
	}
	if user.IsLocal {
		log.Printf("Denied account %s is a local account, ignoring its sssd deny entry", username)
		return UserInfo{}, false
	}

	user.IsLocked = true
	return user, true
}

// buildUsers merges the NSS entries with the local accounts, the deny list and
// the active sessions into the list the server receives.
//
// The same name can exist in /etc/passwd and in the directory. Logins resolve
// through nsswitch order (files before sss), so the local account is kept and
// the directory duplicate is dropped.
func buildUsers(entries []string, localAccounts map[string]int, denied map[string]bool, loggedInUsers map[string]time.Time) []UserInfo {
	users := make([]UserInfo, 0, len(denied))
	index := make(map[string]int)

	for _, entry := range entries {
		fields := strings.Split(entry, ":")
		if len(fields) < 7 {
			continue
		}

		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}

		username := fields[0]
		if isServiceAccount(username) {
			continue
		}
		localUID, isLocal := localAccounts[username]
		isLocal = isLocal && localUID == uid
		isLocked := false
		if isLocal {
			isLocked = isUserLocked(username)
		} else {
			isLocked = denied[username]
		}

		user := UserInfo{
			Username: username,
			UID:      uid,
			HomeDir:  fields[5],
			Shell:    fields[6],
			IsLocal:  isLocal,
			IsLocked: isLocked,
		}
		if loginTime, ok := loggedInUsers[username]; ok {
			user.IsLoggedIn = true
			user.LoggedInAt = loginTime.Unix()
		}

		previous, seen := index[username]
		if !seen {
			index[username] = len(users)
			users = append(users, user)
			continue
		}
		if users[previous].IsLocal || !isLocal {
			continue
		}
		users[previous] = user
	}

	return users
}

// nssUser resolves a single account through NSS, whichever source provides it.
func nssUser(username string) (UserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nssTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "getent", "passwd", username).Output()
	if err != nil {
		return UserInfo{}, fmt.Errorf("account %s not found in NSS: %v", username, err)
	}

	fields := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(fields) < 7 {
		return UserInfo{}, fmt.Errorf("unexpected NSS entry for %s: %q", username, strings.TrimSpace(string(out)))
	}
	uid, err := strconv.Atoi(fields[2])
	if err != nil {
		return UserInfo{}, fmt.Errorf("unexpected uid for %s: %q", username, fields[2])
	}

	return UserInfo{
		Username: fields[0],
		UID:      uid,
		HomeDir:  fields[5],
		Shell:    fields[6],
		IsLocal:  isLocalUser(fields[0]),
	}, nil
}

// nssPasswdEntries runs getent passwd, which goes through nsswitch.conf and
// therefore includes SSSD/LDAP accounts when the directory has enumeration
// enabled. /etc/passwd is the fallback so local accounts keep working on hosts
// without getent or when the directory lookup fails.
func nssPasswdEntries() []string {
	ctx, cancel := context.WithTimeout(context.Background(), nssTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "getent", "passwd").Output()
	if err != nil {
		log.Printf("getent passwd failed (%v), falling back to /etc/passwd", err)
		data, readErr := os.ReadFile("/etc/passwd")
		if readErr != nil {
			return nil
		}
		out = data
	}

	var entries []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			entries = append(entries, line)
		}
	}
	return entries
}

// localUIDs maps the accounts in /etc/passwd to their uid. The uid matters:
// a directory account can share a name with a local one (see getUsers).
func localUIDs() map[string]int {
	uids := make(map[string]int)

	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return uids
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) < 3 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		if _, ok := uids[fields[0]]; !ok {
			uids[fields[0]] = uid
		}
	}
	return uids
}

// isLocalUser reports whether usermod can manage the account, i.e. whether
// /etc/passwd holds an entry with that name.
func isLocalUser(username string) bool {
	_, ok := localUIDs()[username]
	return ok
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

// desktopUser picks the desktop user from the graphical sessions, skipping the
// service accounts the agent does not manage.
func desktopUser(sessions []graphicalSession) string {
	for _, session := range sessions {
		if !isServiceAccount(session.username) {
			return session.username
		}
	}
	return ""
}

func getCurrentDesktopUser() string {
	return desktopUser(getGraphicalSessions())
}

// getLocalIP returns the address the server should use to reach this host.
//
// The route towards the server decides: connecting a UDP socket sends no
// packets, but makes the kernel pick the source address it would use, which is
// by definition an address the server can reach. Merely scanning interfaces
// instead reports addresses the host happens to own, e.g. IPv4 link-local
// autoconfiguration (169.254.0.0/16) on a NIC that never got a DHCP lease, or a
// docker bridge. The scan is the fallback for a server that is not routable.
func getLocalIP(serverURL string) string {
	if ip := routeSourceIP(serverURL); ip != "" {
		return ip
	}
	return scanIPv4()
}

// routeSourceIP returns the local address of the route to the server.
func routeSourceIP(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}

	conn, err := net.Dial("udp", net.JoinHostPort(u.Hostname(), "9"))
	if err != nil {
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	ip := usableIPv4(addr.IP)
	if ip == nil {
		return ""
	}
	return ip.String()
}

// scanIPv4 is the fallback for a server that cannot be reached: the first
// usable address on a physical interface, then on a container/VM interface
// (reachable only through a bridge or tunnel, but better than nothing).
func scanIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, virtual := range []bool{false, true} {
		for _, iface := range ifaces {
			if isVirtualInterface(iface.Name) != virtual {
				continue
			}
			if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				if ip := usableIPv4(ipnet.IP); ip != nil {
					return ip.String()
				}
			}
		}
	}
	return ""
}

// usableIPv4 rejects addresses that do not identify this host to the server:
// IPv6 entries, loopback, and link-local autoconfiguration (169.254.0.0/16 on
// a NIC whose DHCP lease failed).
func usableIPv4(ip net.IP) net.IP {
	ip4 := ip.To4()
	if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
		return nil
	}
	return ip4
}

// isVirtualInterface reports whether the interface belongs to a container or
// virtualization stack; its addresses do not identify the host on the network.
func isVirtualInterface(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "vnet", "tun", "tap", "wg"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
