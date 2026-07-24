#!/usr/bin/env bash
# Build the alarm clock and install it as a systemd kiosk service.
# Run this ON the Raspberry Pi (Gio needs cgo, so build natively).
set -euo pipefail

cd "$(dirname "$0")/.."

PREFIX=${PREFIX:-/usr/local}
USER_NAME=${SUDO_USER:-$USER}
UID_NUM=$(id -u "$USER_NAME")

# System dependencies (Debian/Raspberry Pi OS). Set SKIP_APT=1 to skip.
# librespot is installed separately (see deploy/README.md); it is optional.
if [ "${SKIP_APT:-0}" != "1" ] && command -v apt-get >/dev/null 2>&1; then
	echo "==> Installing system dependencies…"
	sudo apt-get update
	sudo apt-get install -y \
		gcc pkg-config \
		libwayland-dev libxkbcommon-dev libxkbcommon-x11-dev \
		libx11-dev libx11-xcb-dev \
		libegl1-mesa-dev libgles2-mesa-dev libffi-dev libxcursor-dev libvulkan-dev \
		sway mpv libasound2-dev
fi

echo "==> Building alarmclock…"
go build -o alarmclock ./cmd/alarmclock

echo "==> Installing binary to $PREFIX/bin…"
sudo install -Dm755 alarmclock "$PREFIX/bin/alarmclock"

echo "==> Installing sway kiosk config…"
sudo install -Dm644 deploy/sway/config /etc/alarmclock/sway.config

echo "==> Installing systemd service…"
sudo install -Dm644 deploy/alarmclock.service /etc/systemd/system/alarmclock.service
sudo sed -i "s/^User=.*/User=$USER_NAME/" /etc/systemd/system/alarmclock.service
sudo sed -i "s#XDG_RUNTIME_DIR=/run/user/1000#XDG_RUNTIME_DIR=/run/user/$UID_NUM#" /etc/systemd/system/alarmclock.service

sudo systemctl daemon-reload
sudo systemctl enable alarmclock

echo
echo "Installed. Start now with:  sudo systemctl start alarmclock"
echo "Logs:                       journalctl -u alarmclock -f"
