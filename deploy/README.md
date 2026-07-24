# Deploying on the Raspberry Pi 5

The app is built **natively on the Pi** (Gio needs cgo on Linux, which makes
cross-compiling from macOS painful). It runs fullscreen as a systemd service
under **sway** as a single-application kiosk compositor (which also rotates the
display) — no desktop required.

## 1. System dependencies

Raspberry Pi OS (Bookworm, 64-bit):

```sh
# Go toolchain (or install the latest from go.dev)
sudo apt update
sudo apt install -y golang

# Gio build dependencies
sudo apt install -y gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
  libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev \
  libxcursor-dev libvulkan-dev

# Kiosk compositor + audio
sudo apt install -y sway mpv
```

**librespot** (Spotify Connect device) isn't in apt; install a binary or build it:

```sh
# Build dependency for librespot's default ALSA backend (provides alsa.pc):
sudo apt install -y libasound2-dev

# Build via cargo. Use --locked: cargo install otherwise re-resolves build
# dependencies to their newest versions, which currently pulls an incompatible
# vergen-lib and breaks librespot-core's build script.
cargo install librespot --locked
```

Requires a recent Rust toolchain (install via https://rustup.rs; the apt
`rustc` is usually too old). Alternatively, download a prebuilt `aarch64`
binary from https://github.com/librespot-org/librespot/releases into
`/usr/local/bin/librespot`.

## 2. Display orientation

The Touch Display 2 is 720×1280 native; the app runs **landscape 1280×720**.

**Rotate at the compositor level, not the kernel.** If you rotate with a
`video=…,rotate=` line in `config.txt` (KMS), the picture rotates but pointer
and touch input do *not* — the cursor/touch ends up 90° off from what you see.
Rotate in the Wayland compositor so rendering and input rotate together.

`cage` cannot rotate its output, so this deployment uses **sway** as the kiosk
compositor (`sudo apt install -y sway`). The service launches
`sway -c /etc/alarmclock/sway.config`, installed from `deploy/sway/config`:

```
output DSI-2 transform 270
default_border none
xwayland disable
exec /usr/local/bin/alarmclock
```

This rotates the display to landscape and keeps pointer/touch input aligned.
Confirm your output name and orientation with `wlr-randr`; if they differ, edit
`deploy/sway/config` (output name / `transform 90|270`) and re-run the install.

## 3. Spotify (optional)

1. Create a Spotify **developer app**; note the **Client ID** and add the
   redirect URI `http://127.0.0.1:8888/callback`.
2. Put the Client ID in the config (`~/.config/alarmclock/config.json` →
   `spotify.client_id`) or export `ALARMCLOCK_SPOTIFY_CLIENT_ID`.
3. First run: open **Spotify → Verbind met Spotify**, authorize in the browser.
4. Open Spotify on your phone once and select the **"Wekker"** device so it
   registers with your account (needed for Web-API playback).

Spotify requires a **Premium** account.

## 4. Build & install

```sh
git clone <repo> alarm-clock && cd alarm-clock
./deploy/install.sh          # builds, installs the binary + service, enables it
sudo systemctl start alarmclock
journalctl -u alarmclock -f  # follow logs
```

The service restarts on crash and starts on boot. To update: `git pull` then
re-run `./deploy/install.sh` and `sudo systemctl restart alarmclock`.

## Troubleshooting

- **Black screen / sway won't start**: ensure the service runs on the seat that
  owns the display (`TTYPath`), and that the user is in the `video`/`render`
  groups. Check `journalctl -u alarmclock`.
- **No sound**: verify the output with `mpv <some.mp3>`; set the default sink
  with `wpctl`/`raspi-config`. The alarm tone is generated on first run at
  `~/.cache/alarmclock/alarm.wav`.
- **Spotify "device not found"**: activate the "Wekker" device once from the
  Spotify phone app; confirm librespot is running (`pgrep librespot`).
