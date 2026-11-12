package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

type CommandResult struct {
	ComputerID int    `json:"computer_id"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}

// executeBatch runs the commands of one heartbeat. Commands that change this
// machine's sssd deny list are collected first and applied with a single write
// and a single sssd restart: every restart counts against the unit's start limit
// (Debian and Ubuntu ship sssd.service with 5 starts per 50s), so disabling
// several directory accounts in a row used to leave sssd failed with
// start-limit-hit and every later change rejected with it. Commands that touch
// nothing shared run one by one as before.
func executeBatch(cmds []Command) []CommandResult {
	results := make([]CommandResult, len(cmds))

	type denyChange struct {
		index    int
		username string
		deny     bool
	}

	// The list on disk is the starting point, so a hand-edited snippet is kept
	// and only the batch's own changes are applied.
	want := deniedUsernames()
	changed := false
	handled := make([]bool, len(cmds))
	var changes []denyChange

	for i, cmd := range cmds {
		username, deny, ok := denyListCommand(cmd)
		if !ok {
			continue
		}

		handled[i] = true
		changes = append(changes, denyChange{index: i, username: username, deny: deny})
		if deny && !want[username] {
			want[username] = true
			changed = true
		}
		if !deny && want[username] {
			delete(want, username)
			changed = true
		}
	}

	// A batch that disables and enables the same account again leaves the list
	// as it was; rewriting it would only restart sssd for nothing.
	var applyErr error
	if changed {
		applyErr = applyDenyList(want)
	}

	// The deny list goes in before the rest of the batch, so a user who is
	// disabled and logged out by the same heartbeat does not get a session in
	// between.
	for i, cmd := range cmds {
		if !handled[i] {
			results[i] = execute(cmd)
		}
	}

	for _, change := range changes {
		if change.deny {
			results[change.index] = deniedResult(change.username, applyErr)
		} else {
			results[change.index] = allowedResult(change.username, applyErr)
		}
	}
	return results
}

// denyListCommand reports the account and direction of a command that changes
// the sssd deny list. Local accounts are locked with usermod and service accounts
// are refused, so both are left to execute.
func denyListCommand(cmd Command) (username string, deny bool, ok bool) {
	if isServiceAccount(cmd.TargetUser) || isLocalUser(cmd.TargetUser) {
		return "", false, false
	}

	switch cmd.Type {
	case "disable_user":
		return cmd.TargetUser, true, true
	case "enable_user":
		return cmd.TargetUser, false, true
	}
	return "", false, false
}

// execute runs one command from the server. A command that names an account the
// agent does not manage is refused: such an account is not reported either, so
// the only way one arrives is a row left over on the server, or a hand-made
// request.
func execute(cmd Command) CommandResult {
	if isServiceAccount(cmd.TargetUser) {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("user %s is not managed by the agent (service accounts are ignored)", cmd.TargetUser),
		}
	}

	switch cmd.Type {
	case "disable_user":
		return runDisableUser(cmd.TargetUser)
	case "enable_user":
		return runEnableUser(cmd.TargetUser)
	case "logout_user":
		return runLogoutUser(cmd.TargetUser)
	case "restart_dm":
		return runRestartDM()
	default:
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("unknown command type: %s", cmd.Type),
		}
	}
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
	return deniedResult(username, denyDirectoryUser(username))
}

// deniedResult reports the outcome of a deny list that has already been written
// and applied. A written configuration is not proof that logins are refused, so
// sssd is asked through PAM and its answer is surfaced when the account can
// still log in.
func deniedResult(username string, err error) CommandResult {
	if err != nil {
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to disable user %s: %v", username, err),
		}
	}

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
	return allowedResult(username, allowDirectoryUser(username))
}

// allowedResult reports the outcome of a deny list that has already been written
// and applied.
func allowedResult(username string, err error) CommandResult {
	if err != nil {
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

// runLogoutUser ends every session the user has on this machine.
//
// This goes through logind rather than signalling every process of the user's
// uid (pkill -KILL -u): terminate-user is scoped to the user's sessions, so it
// ends exactly the sessions loginctl list-sessions reports - the desktop
// session on the console (X11 or Wayland), RDP, ssh - and deallocates them, so
// the display manager or the remote stack sees the session end. Processes of
// that user running outside a session (a lingering user service, for instance)
// keep running; kill-user is the operation that reaches everything.
func runLogoutUser(username string) CommandResult {
	out, err := runCommand("loginctl", "terminate-user", username)
	if err != nil {
		// logind answers "is not logged in or lingering" when the user has no
		// session to terminate, which is not a failure of the request.
		if strings.Contains(string(out), "not logged in") {
			return CommandResult{
				Success: true,
				Message: fmt.Sprintf("User %s had no active sessions", username),
			}
		}
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to logout user %s: %s (%v)", username, strings.TrimSpace(string(out)), err),
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
