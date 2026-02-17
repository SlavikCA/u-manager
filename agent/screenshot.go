package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
)

type graphicalSession struct {
	username string
	display  string
	date     string // raw date string for sorting
}

// getGraphicalSessions parses `who` output for entries with (:<N>) displays,
// returning sessions sorted by most recent first.
func getGraphicalSessions() []graphicalSession {
	out, err := exec.Command("who").Output()
	if err != nil {
		return nil
	}

	displayRe := regexp.MustCompile(`\(:(\d+)\)`)
	var sessions []graphicalSession

	for _, line := range strings.Split(string(out), "\n") {
		matches := displayRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sessions = append(sessions, graphicalSession{
			username: fields[0],
			display:  ":" + matches[1],
			date:     strings.Join(fields[2:len(fields)-1], " "),
		})
	}

	// Sort by date descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].date > sessions[j].date
	})

	return sessions
}

func setXauthForUser(username string) {
	u, err := user.Lookup(username)
	if err != nil {
		return
	}
	xauth := fmt.Sprintf("/run/user/%s/.Xauthority", u.Uid)
	if _, err := os.Stat(xauth); err != nil {
		xauth = filepath.Join(u.HomeDir, ".Xauthority")
	}
	os.Setenv("XAUTHORITY", xauth)
}

func captureScreenshot() ([]byte, error) {
	sessions := getGraphicalSessions()
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no graphical sessions found in 'who' output")
	}

	for _, sess := range sessions {
		os.Setenv("DISPLAY", sess.display)
		setXauthForUser(sess.username)

		n := screenshot.NumActiveDisplays()
		if n == 0 {
			continue
		}

		img, err := screenshot.CaptureDisplay(0)
		if err != nil {
			continue
		}

		var buf bytes.Buffer
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
		if err != nil {
			return nil, fmt.Errorf("jpeg encode failed: %w", err)
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("no active displays found (tried %d session(s))", len(sessions))
}

func sendScreenshot(cfg *Config, data []byte) {
	url := fmt.Sprintf("%s/api/agent/screenshot?computer_id=%d", cfg.ServerURL, cfg.ComputerID)

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to create screenshot request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("Authorization", "Bearer "+cfg.ApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Screenshot upload failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Screenshot upload returned status %d", resp.StatusCode)
	}
}

func startScreenshotLoop(cfg *Config) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		data, err := captureScreenshot()
		if err != nil {
			log.Printf("Screenshot capture failed: %v", err)
			continue
		}
		sendScreenshot(cfg, data)
	}
}
