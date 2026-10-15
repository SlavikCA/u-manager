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
	ComputerID int    `json:"computer_id"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
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
	case "restart_dm":
		result = runRestartDM()
	default:
		result = CommandResult{
			Success: false,
			Error:   fmt.Sprintf("unknown command type: %s", cmd.Type),
		}
	}

	result.ComputerID = cfg.ComputerID
	log.Printf("Command %d (%s %s): success=%v", cmd.ID, cmd.Type, cmd.TargetUser, result.Success)

	reportResult(cfg, cmd.ID, result)
}

func runDisableUser(username string) CommandResult {
	if !isLocalUser(username) {
		return denyDirectoryLogins(username)
	}

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
	if !isLocalUser(username) {
		return allowDirectoryLogins(username)
	}

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

// denyDirectoryLogins disables a directory (SSSD/LDAP, e.g. Authentik) account
// by adding it to this machine's SSSD deny list: usermod only edits
// /etc/passwd, so it cannot touch an account that lives in the directory.
func denyDirectoryLogins(username string) CommandResult {
	if err := denyDirectoryUser(username); err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to disable user %s: %v", username, err),
		}
	}

	// A written configuration is not proof that logins are refused, so ask sssd
	// through PAM and surface its answer when the account can still log in.
	blocked, verified, output := loginsDenied(username)
	if verified && !blocked {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("deny list written and sssd restarted, but the PAM account check still accepts %s: %s", username, output),
		}
	}

	message := fmt.Sprintf("User %s has been disabled on this machine (sssd deny list)", username)
	if !verified {
		message = fmt.Sprintf("%s - logins could not be verified through PAM: %s", message, output)
	}
	return CommandResult{
		Success: true,
		Message: message,
	}
}

// allowDirectoryLogins re-enables a directory account by removing it from this
// machine's SSSD deny list. The deny list is used rather than NSS, so an account
// the directory stopped returning can still be enabled again.
func allowDirectoryLogins(username string) CommandResult {
	if err := allowDirectoryUser(username); err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to enable user %s: %v", username, err),
		}
	}

	return CommandResult{
		Success: true,
		Message: fmt.Sprintf("User %s can log in to this machine again", username),
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

func runRestartDM() CommandResult {
	out, err := exec.Command("sudo", "systemctl", "restart", "lightdm.service").CombinedOutput()
	if err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to restart display manager: %s (%v)", string(out), err),
		}
	}
	return CommandResult{
		Success: true,
		Message: "Display manager restarted successfully",
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
