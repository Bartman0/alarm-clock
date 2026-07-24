#!/usr/bin/env bash
# Build the alarm clock and install it as a systemd kiosk service.
# Run this ON the Raspberry Pi (Gio needs cgo, so build natively).
set -euo pipefail

cd "$(dirname "$0")/.."

PREFIX=${PREFIX:-/usr/local}
USER_NAME=${SUDO_USER:-$USER}
UID_NUM=$(id -u "$USER_NAME")

echo "==> Building alarmclock…"
go build -o alarmclock ./cmd/alarmclock

echo "==> Installing binary to $PREFIX/bin…"
sudo install -Dm755 alarmclock "$PREFIX/bin/alarmclock"

echo "==> Installing systemd service…"
sudo install -Dm644 deploy/alarmclock.service /etc/systemd/system/alarmclock.service
sudo sed -i "s/^User=.*/User=$USER_NAME/" /etc/systemd/system/alarmclock.service
sudo sed -i "s#XDG_RUNTIME_DIR=/run/user/1000#XDG_RUNTIME_DIR=/run/user/$UID_NUM#" /etc/systemd/system/alarmclock.service

sudo systemctl daemon-reload
sudo systemctl enable alarmclock

echo
echo "Installed. Start now with:  sudo systemctl start alarmclock"
echo "Logs:                       journalctl -u alarmclock -f"
