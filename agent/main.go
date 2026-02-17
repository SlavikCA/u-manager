package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const agentVersion = "1.9.0"

func main() {
	serverURL := flag.String("server-url", "", "Server URL for registration (e.g. http://192.168.0.230:3000)")
	token := flag.String("token", "", "One-time registration token")
	screenshots := flag.Bool("screenshots", false, "Enable screenshot capture (default: false)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Registration mode: register and exit
	if *token != "" && *serverURL != "" {
		_, err := register(*serverURL, *token, *screenshots)
		if err != nil {
			log.Fatalf("Registration failed: %v", err)
		}
		if err := exec.Command("systemctl", "enable", "lum-agent").Run(); err != nil {
			fmt.Println("Could not enable service automatically. Run manually:")
			fmt.Println("  sudo systemctl enable lum-agent")
			fmt.Println("  sudo systemctl restart lum-agent")
			os.Exit(0)
		}
		if err := exec.Command("systemctl", "restart", "lum-agent").Run(); err != nil {
			fmt.Println("Could not start service automatically. Run manually:")
			fmt.Println("  sudo systemctl restart lum-agent")
			os.Exit(0)
		}
		fmt.Println("Service enabled and started.")
		os.Exit(0)
	}

	// Service mode: load config and run
	cfg, err := loadConfig()
	if err != nil {
		fmt.Println("Linux User Manager Agent v" + agentVersion)
		fmt.Println()
		fmt.Println("No configuration found. To register this agent:")
		fmt.Println("  sudo lum-agent --server-url http://SERVER:3000 --token YOUR_TOKEN")
		fmt.Println()
		fmt.Println("Get a registration token from the web UI at /tokens")
		os.Exit(1)
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

	// Start screenshot loop in background if enabled
	if cfg.Screenshots {
		go startScreenshotLoop(cfg)
	}

	// Start heartbeat loop (blocks forever)
	startHeartbeatLoop(cfg)
}
