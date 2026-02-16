package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"net/http"
	"time"

	"github.com/kbinani/screenshot"
)

func captureScreenshot() ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no active displays found")
	}

	img, err := screenshot.CaptureDisplay(0)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	if err != nil {
		return nil, fmt.Errorf("jpeg encode failed: %w", err)
	}

	return buf.Bytes(), nil
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
