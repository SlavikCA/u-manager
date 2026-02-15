#!/bin/bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root: sudo bash install.sh"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Linux User Manager Agent Installer ==="
echo

# Build
echo "Building agent..."
cd "$SCRIPT_DIR"
go build -o lum-agent .
echo "Build successful."

# Install binary
echo "Installing to /usr/local/bin/lum-agent..."
cp lum-agent /usr/local/bin/lum-agent
chmod 755 /usr/local/bin/lum-agent

# Install service
echo "Installing systemd service..."
cp lum-agent.service /etc/systemd/system/lum-agent.service
systemctl daemon-reload

# Create config directory
mkdir -p /etc/linux-user-manager

# Check if already registered
if [ -f /etc/linux-user-manager/config.json ]; then
  echo
  echo "Agent is already registered. Starting service..."
  systemctl enable lum-agent
  systemctl restart lum-agent
  echo "Done! Check status with: systemctl status lum-agent"
  exit 0
fi

# Registration
echo
read -p "Server URL (e.g. http://192.168.0.230:3000): " SERVER_URL
read -p "Registration token: " TOKEN

if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
  echo "Server URL and token are required."
  exit 1
fi

echo "Registering agent..."
/usr/local/bin/lum-agent --server-url "$SERVER_URL" --token "$TOKEN" &
AGENT_PID=$!

# Wait a moment for registration
sleep 3

# Check if config was created
if [ -f /etc/linux-user-manager/config.json ]; then
  kill $AGENT_PID 2>/dev/null || true
  wait $AGENT_PID 2>/dev/null || true
  echo "Registration successful!"

  systemctl enable lum-agent
  systemctl start lum-agent
  echo
  echo "Agent installed and running!"
  echo "Check status with: systemctl status lum-agent"
  echo "View logs with: journalctl -u lum-agent -f"
else
  kill $AGENT_PID 2>/dev/null || true
  wait $AGENT_PID 2>/dev/null || true
  echo "Registration may have failed. Check the output above."
  echo "You can register manually with:"
  echo "  sudo lum-agent --server-url $SERVER_URL --token YOUR_TOKEN"
  exit 1
fi
