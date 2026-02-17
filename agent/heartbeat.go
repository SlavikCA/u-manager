package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type HeartbeatRequest struct {
	ComputerID         int        `json:"computer_id"`
	Hostname           string     `json:"hostname"`
	IPAddress          string     `json:"ip_address"`
	AgentVersion       string     `json:"agent_version"`
	CurrentDesktopUser string     `json:"current_desktop_user"`
	Users              []UserInfo `json:"users"`
}

type HeartbeatResponse struct {
	Status   string    `json:"status"`
	Commands []Command `json:"commands"`
	Error    string    `json:"error"`
}

type Command struct {
	ID         int    `json:"id"`
	Type       string `json:"type"`
	TargetUser string `json:"target_user"`
}

var unauthorizedCount int
var executedCommands = make(map[int]bool)

func startHeartbeatLoop(cfg *Config) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Send first heartbeat immediately
	sendHeartbeat(cfg)

	for range ticker.C {
		sendHeartbeat(cfg)
	}
}

func sendHeartbeat(cfg *Config) {
	hostname, _ := os.Hostname()

	req := HeartbeatRequest{
		ComputerID:         cfg.ComputerID,
		Hostname:           hostname,
		IPAddress:          getLocalIP(),
		AgentVersion:       agentVersion,
		CurrentDesktopUser: getCurrentDesktopUser(),
		Users:              getUsers(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		log.Printf("Failed to marshal heartbeat: %v", err)
		return
	}

	httpReq, err := http.NewRequest("POST", cfg.ServerURL+"/api/agent/heartbeat", bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to create heartbeat request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.ApiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read heartbeat response: %v", err)
		return
	}

	if resp.StatusCode == 401 {
		unauthorizedCount++
		log.Printf("Unauthorized (401) response from server (%d/5). Check API key.", unauthorizedCount)
		if unauthorizedCount >= 5 {
			log.Printf("Received %d consecutive 401 responses. Stopping agent. Please re-register.", unauthorizedCount)
			os.Exit(3)
		}
		return
	}

	if resp.StatusCode == 404 {
		log.Printf("Computer not found on server. Please re-register the agent.")
		return
	}

	if resp.StatusCode != 200 {
		log.Printf("Heartbeat returned status %d: %s", resp.StatusCode, string(body))
		return
	}

	unauthorizedCount = 0

	var hbResp HeartbeatResponse
	if err := json.Unmarshal(body, &hbResp); err != nil {
		log.Printf("Failed to parse heartbeat response: %v", err)
		return
	}

	newCommandsExecuted := 0
	for _, cmd := range hbResp.Commands {
		if executedCommands[cmd.ID] {
			log.Printf("Skipping already executed command %d (%s %s)", cmd.ID, cmd.Type, cmd.TargetUser)
			continue
		}
		log.Printf("Received command: %s for user %s (id=%d)", cmd.Type, cmd.TargetUser, cmd.ID)
		executeCommand(cfg, cmd)
		executedCommands[cmd.ID] = true
		newCommandsExecuted++
	}

	if newCommandsExecuted > 0 {
		log.Printf("Executed %d command(s), sending immediate heartbeat to update status", newCommandsExecuted)
		sendHeartbeat(cfg)
	}
}
