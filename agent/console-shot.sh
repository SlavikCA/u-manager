#!/usr/bin/env bash
# shot.sh — screenshot of whoever is on the console. Run as root.
set -euo pipefail

die() { echo "shot: $*" >&2; exit 1; }

SESS=$(loginctl list-sessions --no-legend | awk '$4!="-" && $6=="active" {print $1; exit}')
[[ -n ${SESS:-} ]] || die "no active console session"

SUSER=$(loginctl show-session "$SESS" -p Name --value)
PID=$(pgrep -u "$SUSER" -x gnome-shell | head -n1) || die "no gnome-shell for $SUSER"

XENV=$(tr '\0' '\n' < /proc/$PID/environ | grep -E '^(DISPLAY|XAUTHORITY)=')
[[ -n $XENV ]] || die "no X env in $SUSER's gnome-shell (Wayland?)"

OUT=${1:-/tmp/shot-$(date +%Y%m%d-%H%M%S).png}
runuser -u "$SUSER" -- env $XENV scrot -o "$OUT"
echo "$OUT"
