package main

import (
	"bytes"
	_ "embed"
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

//go:embed console-shot.sh
var consoleShotScript string

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

// captureScreenshotConsole takes a screenshot with the loginctl/runuser/scrot
// method: the embedded console-shot.sh is written to a temp file and executed
// as root; the script finds the active console session and captures it as
// that session's user. scrot picks JPEG from the .jpg output name, so the
func captureScreenshotConsole() ([]byte, error) {
	scriptPath, err := writeConsoleShotScript()
	if err != nil {
		return nil, err
	}
	defer os.Remove(scriptPath)

	// Do NOT create the output file: it must be created by scrot running as
	// the session user, so the user owns and can write it.
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("lum-shot-%d-%d.jpg", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(outPath)

	cmd := exec.Command("bash", scriptPath, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("console-shot failed: %v: %s", err, strings.TrimSpace(string(out)))
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot output: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("console-shot produced an empty file")
	}
	return data, nil
}

// writeConsoleShotScript materializes the embedded console-shot.sh as a
// temporary executable file.
func writeConsoleShotScript() (string, error) {
	f, err := os.CreateTemp("", "lum-console-shot-*.sh")
	if err != nil {
		return "", fmt.Errorf("failed to create temp script file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(consoleShotScript); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("failed to write console-shot script: %w", err)
	}
	if err := f.Chmod(0700); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("failed to make console-shot script executable: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("failed to close console-shot script file: %w", err)
	}
	return path, nil
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
	capture := captureScreenshot
	if cfg.ScreenshotMethod == ScreenshotMethodConsole {
		if _, err := exec.LookPath("scrot"); err != nil {
			log.Printf("Screenshot method 'console' requires scrot to be installed; screenshot loop not started")
			return
		}
		capture = captureScreenshotConsole
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		data, err := capture()
		if err != nil {
			log.Printf("Screenshot capture failed: %v", err)
			continue
		}
		sendScreenshot(cfg, data)
	}
}
