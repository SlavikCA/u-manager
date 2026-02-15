#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root: sudo bash install.sh"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ ! -f "$SCRIPT_DIR/lum-agent" ]; then
  echo "Binary not found. Run build.sh first."
  exit 1
fi

echo "Installing binary to /usr/local/bin/lum-agent..."
cp "$SCRIPT_DIR/lum-agent" /usr/local/bin/lum-agent
chmod 755 /usr/local/bin/lum-agent

echo "Installing systemd service..."
cp "$SCRIPT_DIR/lum-agent.service" /etc/systemd/system/lum-agent.service
systemctl daemon-reload

mkdir -p /etc/linux-user-manager

if [ -f /etc/linux-user-manager/config.json ]; then
  echo "Agent is already registered. Restarting service..."
  systemctl enable lum-agent
  systemctl restart lum-agent
  echo "Done! Check status with: systemctl status lum-agent"
else
  echo "Installed. Register the agent with: sudo bash register.sh <server-url> <token>"
fi
