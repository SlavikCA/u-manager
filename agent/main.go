package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const agentVersion = "1.0.0"

func main() {
	serverURL := flag.String("server-url", "", "Server URL for registration (e.g. http://192.168.0.230:3000)")
	token := flag.String("token", "", "One-time registration token")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Try to load existing config
	cfg, err := loadConfig()
	if err != nil {
		// No config exists, try to register
		if *token == "" || *serverURL == "" {
			fmt.Println("Linux User Manager Agent v" + agentVersion)
			fmt.Println()
			fmt.Println("No configuration found. To register this agent:")
			fmt.Println("  sudo lum-agent --server-url http://SERVER:3000 --token YOUR_TOKEN")
			fmt.Println()
			fmt.Println("Get a registration token from the web UI at /tokens")
			os.Exit(1)
		}

		cfg, err = register(*serverURL, *token)
		if err != nil {
			log.Fatalf("Registration failed: %v", err)
		}
	}

	log.Printf("Linux User Manager Agent v%s starting (computer_id=%d, server=%s)", agentVersion, cfg.ComputerID, cfg.ServerURL)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		os.Exit(0)
	}()

	// Start heartbeat loop (blocks forever)
	startHeartbeatLoop(cfg)
}
