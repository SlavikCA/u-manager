package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
)

type CommandResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func executeCommand(cfg *Config, cmd Command) {
	var result CommandResult

	switch cmd.Type {
	case "disable_user":
		result = runDisableUser(cmd.TargetUser)
	case "enable_user":
		result = runEnableUser(cmd.TargetUser)
	case "logout_user":
		result = runLogoutUser(cmd.TargetUser)
	default:
		result = CommandResult{
			Success: false,
			Error:   fmt.Sprintf("unknown command type: %s", cmd.Type),
		}
	}

	log.Printf("Command %d (%s %s): success=%v", cmd.ID, cmd.Type, cmd.TargetUser, result.Success)

	reportResult(cfg, cmd.ID, result)
}

func runDisableUser(username string) CommandResult {
	out, err := exec.Command("usermod", "-L", username).CombinedOutput()
	if err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to disable user %s: %s (%v)", username, string(out), err),
		}
	}
	return CommandResult{
		Success: true,
		Message: fmt.Sprintf("User %s has been disabled", username),
	}
}

func runEnableUser(username string) CommandResult {
	out, err := exec.Command("usermod", "-U", username).CombinedOutput()
	if err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to enable user %s: %s (%v)", username, string(out), err),
		}
	}
	return CommandResult{
		Success: true,
		Message: fmt.Sprintf("User %s has been enabled", username),
	}
}

func runLogoutUser(username string) CommandResult {
	out, err := exec.Command("pkill", "-KILL", "-u", username).CombinedOutput()
	if err != nil {
		// pkill returns exit code 1 if no processes found, which is fine
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return CommandResult{
				Success: true,
				Message: fmt.Sprintf("User %s had no active sessions", username),
			}
		}
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to logout user %s: %s (%v)", username, string(out), err),
		}
	}
	return CommandResult{
		Success: true,
		Message: fmt.Sprintf("User %s has been logged out", username),
	}
}

func reportResult(cfg *Config, commandID int, result CommandResult) {
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal command result: %v", err)
		return
	}

	url := fmt.Sprintf("%s/api/agent/commands/%d/result", cfg.ServerURL, commandID)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to create command result request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.ApiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Printf("Failed to report command result: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Server returned status %d for command result report", resp.StatusCode)
	}
}
