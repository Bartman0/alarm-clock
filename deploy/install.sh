#!/usr/bin/env bash
# Build the alarm clock and set it up as a Wayland kiosk on Raspberry Pi.
# Run this ON the Pi as your normal user (it uses sudo where needed) — Gio
# needs cgo, so build natively.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$(id -u)" = 0 ]; then
	echo "Run this as your normal user (it uses sudo itself), not with sudo." >&2
	exit 1
fi

PREFIX=${PREFIX:-/usr/local}

# 1. System dependencies (Debian/Raspberry Pi OS). Set SKIP_APT=1 to skip.
#    librespot is installed separately (see deploy/README.md); it is optional.
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

# 2. Build. The novulkan tag drops Gio's Vulkan backend, which crashes on the
#    Pi's V3DV driver; Gio then uses OpenGL ES.
echo "==> Building alarmclock…"
go build -tags novulkan -o alarmclock ./cmd/alarmclock

# 3. Install the binary and the sway kiosk config.
echo "==> Installing binary and sway config…"
sudo install -Dm755 alarmclock "$PREFIX/bin/alarmclock"
sudo install -Dm644 deploy/sway/config /etc/alarmclock/sway.config

# 4. Remove any previous systemd kiosk service (superseded by console autostart).
if [ -f /etc/systemd/system/alarmclock.service ]; then
	echo "==> Removing old systemd service…"
	sudo systemctl disable --now alarmclock.service || true
	sudo rm -f /etc/systemd/system/alarmclock.service
	sudo systemctl daemon-reload
fi

# 5. Enable console autologin on tty1 so a login session (with a seat) exists.
if command -v raspi-config >/dev/null 2>&1; then
	echo "==> Enabling console autologin…"
	sudo raspi-config nonint do_boot_behaviour B2
fi

# 5b. Ensure the user can access DRM/input devices (belt-and-suspenders; logind
#     also grants this to the active console session).
sudo usermod -aG video,render,input "$USER" || true

# 6. Autostart sway (and the app) from the tty1 login shell. Launching from a
#    real login session is what gives sway its seat / DRM master.
PROFILE="$HOME/.profile"
MARKER="# >>> alarmclock kiosk >>>"
if ! grep -qF "$MARKER" "$PROFILE" 2>/dev/null; then
	echo "==> Adding kiosk autostart to $PROFILE…"
	cat >>"$PROFILE" <<EOF

$MARKER
if [ -z "\$WAYLAND_DISPLAY" ] && [ "\$(tty)" = "/dev/tty1" ]; then
  exec sway -c /etc/alarmclock/sway.config
fi
# <<< alarmclock kiosk <<<
EOF
fi

echo
echo "Installed. Reboot to launch the kiosk:   sudo reboot"
echo "Update later with:  git pull && ./deploy/install.sh && sudo reboot"
