#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root: sudo bash register.sh <server-url> <token>"
  exit 1
fi

SERVER_URL="$1"
TOKEN="$2"

if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
  echo "Usage: sudo bash register.sh <server-url> <token>"
  echo "  e.g. sudo bash register.sh http://192.168.0.230:3000 abc-123"
  exit 1
fi

if ! command -v lum-agent &>/dev/null; then
  echo "lum-agent not found. Run build.sh and install.sh first."
  exit 1
fi

echo "Registering agent with $SERVER_URL..."
lum-agent --server-url "$SERVER_URL" --token "$TOKEN" &
AGENT_PID=$!

sleep 3

if [ -f /etc/linux-user-manager/config.json ]; then
  kill $AGENT_PID 2>/dev/null || true
  wait $AGENT_PID 2>/dev/null || true
  echo "Registration successful!"

  systemctl enable lum-agent
  systemctl restart lum-agent
  echo
  echo "Agent installed and running!"
  echo "Check status with: systemctl status lum-agent"
  echo "View logs with: journalctl -u lum-agent -f"
else
  kill $AGENT_PID 2>/dev/null || true
  wait $AGENT_PID 2>/dev/null || true
  echo "Registration failed. Check the output above."
  exit 1
fi
