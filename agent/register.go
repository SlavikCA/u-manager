package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type RegisterRequest struct {
	Token        string `json:"token"`
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ip_address"`
	AgentVersion string `json:"agent_version"`
}

type RegisterResponse struct {
	Status     string `json:"status"`
	ComputerID int    `json:"computer_id"`
	ApiKey     string `json:"api_key"`
	Message    string `json:"message"`
	Error      string `json:"error"`
}

func register(serverURL, token string, screenshots bool) (*Config, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	reqBody := RegisterRequest{
		Token:        token,
		Hostname:     hostname,
		IPAddress:    getLocalIP(),
		AgentVersion: agentVersion,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(serverURL+"/api/agent/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var regResp RegisterResponse
	if err := json.Unmarshal(body, &regResp); err != nil {
		return nil, fmt.Errorf("invalid response from server: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registration failed: %s", regResp.Error)
	}

	cfg := &Config{
		ComputerID:  regResp.ComputerID,
		ServerURL:   serverURL,
		ApiKey:      regResp.ApiKey,
		Screenshots: screenshots,
	}

	if err := saveConfig(cfg); err != nil {
		return nil, fmt.Errorf("registered successfully but failed to save config: %w", err)
	}

	fmt.Printf("Registered successfully! Computer ID: %d\n", cfg.ComputerID)
	return cfg, nil
}
