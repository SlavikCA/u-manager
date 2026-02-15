# Linux User Manager - Agent API Documentation

This document describes the API endpoints that agents use to communicate with the Linux User Manager server.

## Base URL

The server URL is configured via the `SERVER_URL` environment variable on the server. All API endpoints are prefixed with `/api/agent`.

Example: If `SERVER_URL=https://manager.example.com`, then the heartbeat endpoint would be:
```
https://manager.example.com/api/agent/heartbeat
```

## Authentication

Agents authenticate using a registration token (one-time use) and then use their assigned `computer_id` for subsequent requests.

---

## Endpoints

### 1. Register Agent

Register a new agent with the server using a one-time token.

**Endpoint:** `POST /api/agent/register`

**Request Body:**
```json
{
  "token": "abc123-def456-ghi789",
  "hostname": "workstation-01",
  "ip_address": "192.168.1.100",
  "agent_version": "1.0.0"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| token | string | Yes | One-time registration token from the web UI |
| hostname | string | Yes | Machine hostname |
| ip_address | string | No | Machine IP address |
| agent_version | string | No | Agent software version |

**Success Response (200):**
```json
{
  "status": "ok",
  "computer_id": 1,
  "message": "Agent registered successfully"
}
```

**Error Responses:**

- `400 Bad Request` - Missing required fields
```json
{
  "error": "Token and hostname are required"
}
```

- `401 Unauthorized` - Invalid or already used token
```json
{
  "error": "Invalid or already used token"
}
```

---

### 2. Heartbeat

Send periodic heartbeat to report status and receive pending commands. **Should be called every 10 seconds.**

**Endpoint:** `POST /api/agent/heartbeat`

**Request Body:**
```json
{
  "computer_id": 1,
  "hostname": "workstation-01",
  "ip_address": "192.168.1.100",
  "agent_version": "1.0.0",
  "current_desktop_user": "john",
  "users": [
    {
      "username": "john",
      "uid": 1000,
      "home_dir": "/home/john",
      "shell": "/bin/bash",
      "is_locked": false,
      "is_logged_in": true
    },
    {
      "username": "jane",
      "uid": 1001,
      "home_dir": "/home/jane",
      "shell": "/bin/bash",
      "is_locked": false,
      "is_logged_in": false
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| computer_id | integer | Yes | Computer ID from registration |
| hostname | string | No | Machine hostname |
| ip_address | string | No | Current IP address |
| agent_version | string | No | Agent software version |
| current_desktop_user | string | No | Username of currently logged-in desktop user (null if none) |
| users | array | No | List of users with UID >= 1000 |

**User Object:**

| Field | Type | Description |
|-------|------|-------------|
| username | string | Linux username |
| uid | integer | User ID (should be >= 1000) |
| home_dir | string | Home directory path |
| shell | string | Default shell |
| is_locked | boolean | Whether account is locked (check with `passwd -S`) |
| is_logged_in | boolean | Whether user has active sessions |

**Success Response (200):**
```json
{
  "status": "ok",
  "commands": [
    {
      "id": 123,
      "type": "disable_user",
      "target_user": "john"
    }
  ]
}
```

**Error Responses:**

- `400 Bad Request` - Missing computer_id
```json
{
  "error": "computer_id is required"
}
```

- `404 Not Found` - Computer not registered
```json
{
  "error": "Computer not found. Please re-register the agent."
}
```

---

### 3. Report Command Result

Report the result of executing a command.

**Endpoint:** `POST /api/agent/commands/:id/result`

**URL Parameters:**
- `id` - Command ID from the heartbeat response

**Request Body (Success):**
```json
{
  "success": true,
  "message": "User john has been disabled"
}
```

**Request Body (Failure):**
```json
{
  "success": false,
  "error": "Permission denied: unable to modify user"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| success | boolean | Yes | Whether command executed successfully |
| message | string | No | Success message |
| error | string | No | Error message if failed |

**Success Response (200):**
```json
{
  "status": "ok"
}
```

**Error Response:**

- `404 Not Found` - Command not found
```json
{
  "error": "Command not found"
}
```

---

## Command Types

The server may send the following command types:

### disable_user

Lock a user account to prevent login.

```json
{
  "id": 123,
  "type": "disable_user",
  "target_user": "john"
}
```

**Recommended Linux command:**
```bash
usermod -L <username>
```

### enable_user

Unlock a user account to allow login.

```json
{
  "id": 124,
  "type": "enable_user",
  "target_user": "john"
}
```

**Recommended Linux command:**
```bash
usermod -U <username>
```

### logout_user

Force logout a user (terminate all their sessions).

```json
{
  "id": 125,
  "type": "logout_user",
  "target_user": "john"
}
```

**Recommended Linux command:**
```bash
pkill -KILL -u <username>
```

---

## Agent Implementation Guide

### Startup Flow

1. Check if `computer_id` is stored locally (e.g., in `/etc/linux-user-manager/config`)
2. If not registered:
   - Prompt for registration token
   - Call `POST /api/agent/register`
   - Store returned `computer_id` locally
3. Start heartbeat loop

### Heartbeat Loop

```
every 10 seconds:
    1. Gather system information:
       - Get current desktop user (check display manager or `who`)
       - Get list of users with UID >= 1000 from /etc/passwd
       - Check lock status for each user (passwd -S)
       - Check login status for each user (who, w, or loginctl)
    
    2. Send POST /api/agent/heartbeat
    
    3. Process any commands in response:
       for each command:
           - Execute the command
           - Report result via POST /api/agent/commands/:id/result
```

### Getting Users from /etc/passwd

```bash
# Get users with UID >= 1000 (excluding nobody/nogroup)
awk -F: '$3 >= 1000 && $3 < 65534 {print $1":"$3":"$6":"$7}' /etc/passwd
```

### Checking if User is Locked

```bash
# Check user lock status
passwd -S username
# Output: username L ... (L = locked, P = has password)
```

### Getting Current Desktop User

```bash
# Method 1: Check who is on display :0
who | grep ':0' | awk '{print $1}'

# Method 2: Using loginctl (systemd)
loginctl list-sessions --no-legend | while read session uid user seat tty; do
  if [ "$seat" = "seat0" ]; then
    echo $user
    break
  fi
done
```

### Checking if User is Logged In

```bash
# Check if user has any sessions
who | grep -q "^username " && echo "logged in"

# Or using loginctl
loginctl list-users --no-legend | grep -q "username"
```

---

## Example Go Agent Structure

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "strings"
    "time"
)

const (
    ServerURL     = "https://manager.example.com"
    ConfigFile    = "/etc/linux-user-manager/config"
    PollInterval  = 10 * time.Second
)

type Config struct {
    ComputerID int    `json:"computer_id"`
    ServerURL  string `json:"server_url"`
}

type User struct {
    Username   string `json:"username"`
    UID        int    `json:"uid"`
    HomeDir    string `json:"home_dir"`
    Shell      string `json:"shell"`
    IsLocked   bool   `json:"is_locked"`
    IsLoggedIn bool   `json:"is_logged_in"`
}

type HeartbeatRequest struct {
    ComputerID        int    `json:"computer_id"`
    Hostname          string `json:"hostname"`
    IPAddress         string `json:"ip_address"`
    AgentVersion      string `json:"agent_version"`
    CurrentDesktopUser string `json:"current_desktop_user"`
    Users             []User `json:"users"`
}

type Command struct {
    ID         int    `json:"id"`
    Type       string `json:"type"`
    TargetUser string `json:"target_user"`
}

type HeartbeatResponse struct {
    Status   string    `json:"status"`
    Commands []Command `json:"commands"`
}

func main() {
    config := loadOrRegister()
    
    for {
        heartbeat(config)
        time.Sleep(PollInterval)
    }
}

func heartbeat(config *Config) {
    users := getUsers()
    currentUser := getCurrentDesktopUser()
    hostname, _ := os.Hostname()
    
    req := HeartbeatRequest{
        ComputerID:        config.ComputerID,
        Hostname:          hostname,
        AgentVersion:      "1.0.0",
        CurrentDesktopUser: currentUser,
        Users:             users,
    }
    
    resp, err := postJSON(config.ServerURL+"/api/agent/heartbeat", req)
    if err != nil {
        fmt.Printf("Heartbeat error: %v\n", err)
        return
    }
    
    var heartbeatResp HeartbeatResponse
    json.Unmarshal(resp, &heartbeatResp)
    
    for _, cmd := range heartbeatResp.Commands {
        executeCommand(config, cmd)
    }
}

func executeCommand(config *Config, cmd Command) {
    var err error
    
    switch cmd.Type {
    case "disable_user":
        err = exec.Command("usermod", "-L", cmd.TargetUser).Run()
    case "enable_user":
        err = exec.Command("usermod", "-U", cmd.TargetUser).Run()
    case "logout_user":
        err = exec.Command("pkill", "-KILL", "-u", cmd.TargetUser).Run()
    }
    
    // Report result
    result := map[string]interface{}{
        "success": err == nil,
    }
    if err != nil {
        result["error"] = err.Error()
    }
    
    postJSON(fmt.Sprintf("%s/api/agent/commands/%d/result", config.ServerURL, cmd.ID), result)
}

// ... implement other functions
```

---

## Security Recommendations

1. **Use HTTPS**: Always use HTTPS for communication
2. **Verify certificates**: Don't skip TLS certificate verification
3. **Run as root**: Agent needs root privileges to manage users
4. **Secure config file**: Store config with restricted permissions (600)
5. **Log actions**: Keep local logs of all actions performed

---

## Systemd Service File

```ini
[Unit]
Description=Linux User Manager Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lum-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Save as `/etc/systemd/system/lum-agent.service` and enable with:
```bash
systemctl enable lum-agent
systemctl start lum-agent
```
