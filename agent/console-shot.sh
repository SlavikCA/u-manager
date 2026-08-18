#!/usr/bin/env bash
# Screenshot of whoever is currently on the console.
# Run as root:  sudo ./console-shot.sh [output-file]
set -euo pipefail

# 1) Find the active graphical session (exclude the GDM greeter)
SESS=$(loginctl list-sessions --no-legend | awk '$6=="active" && $3!="gdm" {print $1; exit}')
[[ -n "${SESS:-}" ]] || { echo "no active user session" >&2; exit 1; }

# 2) Which user owns it
SUSER=$(loginctl show-session "$SESS" -p Name --value)

# 3) Read that user's DISPLAY and XAUTHORITY from gnome-shell's environment
PID=$(pgrep -u "$SUSER" -x gnome-shell | head -n1)
ENVPAIRS=$(tr '\0' '\n' < "/proc/$PID/environ" | grep -E '^(DISPLAY|XAUTHORITY)=')

# 4) Take the shot as that user
OUT="${1:-/tmp/shot-$(date +%Y%m%d-%H%M%S).png}"

runuser -u "$SUSER" -- env $ENVPAIRS scrot -o "$OUT"

echo "Saved $OUT (user: $SUSER, session: $SESS)"
