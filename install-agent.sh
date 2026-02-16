#!/bin/bash
set -e

REPO="SlavikCA/u-manager"
VERSION="${LUM_VERSION:-latest}"

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root: curl -fsSL URL | sudo bash"
  exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Resolve version
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
  if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest release version."
    exit 1
  fi
fi

BINARY="lum-agent-linux-$GOARCH"
URL="https://github.com/$REPO/releases/download/$VERSION/$BINARY"

echo "=== Linux User Manager Agent Installer ==="
echo "Version: $VERSION"
echo "Architecture: $GOARCH"
echo

# Download binary
UPGRADE=false
if [ -f /usr/local/bin/lum-agent ]; then
  UPGRADE=true
fi

if [ "$UPGRADE" = true ]; then
  echo "Stopping lum-agent service..."
  systemctl stop lum-agent || true
fi

echo "Downloading $URL..."
curl -fSL -o /usr/local/bin/lum-agent "$URL"
chmod 755 /usr/local/bin/lum-agent

# Install systemd service
echo "Installing systemd service..."
cat > /etc/systemd/system/lum-agent.service <<EOF
[Unit]
Description=Linux User Manager Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lum-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
mkdir -p /etc/linux-user-manager

echo
if [ "$UPGRADE" = true ]; then
  echo "Upgrade complete!"
  echo
  echo "Restarting lum-agent service..."
  systemctl restart lum-agent
  echo "Service restarted successfully."
else
  echo "Installation complete!"
  echo
  echo "To register this agent, run:"
  echo "  sudo lum-agent --server-url http://YOUR_SERVER:3000 --token YOUR_TOKEN"
fi
