package main

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type UserInfo struct {
	Username   string `json:"username"`
	UID        int    `json:"uid"`
	HomeDir    string `json:"home_dir"`
	Shell      string `json:"shell"`
	IsLocked   bool   `json:"is_locked"`
	IsLoggedIn bool   `json:"is_logged_in"`
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
		users = append(users, UserInfo{
			Username:   username,
			UID:        uid,
			HomeDir:    fields[5],
			Shell:      fields[6],
			IsLocked:   isUserLocked(username),
			IsLoggedIn: loggedInUsers[username],
		})
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

func getLoggedInUsers() map[string]bool {
	result := make(map[string]bool)
	out, err := exec.Command("who").Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			result[fields[0]] = true
		}
	}
	return result
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
