#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "Building agent..."
go build -o lum-agent .
echo "Build successful: $SCRIPT_DIR/lum-agent"
