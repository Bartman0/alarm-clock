# Deploying on the Raspberry Pi 5

The app is built **natively on the Pi** (Gio needs cgo on Linux, which makes
cross-compiling from macOS painful). It runs fullscreen under **sway** as a
single-application kiosk compositor (which also rotates the display) — no
desktop required. sway is launched from the **tty1 login shell** via console
autologin; that login session owns the seat, so sway gets DRM/input access (a
plain systemd service does not, which is why we don't use one).

## 1. System dependencies

`./deploy/install.sh` installs everything below automatically via apt (except
librespot). Run these manually only if you set `SKIP_APT=1` or want them ahead
of time. Raspberry Pi OS (Bookworm, 64-bit):

```sh
# Go toolchain (or install the latest from go.dev)
sudo apt update
sudo apt install -y golang

# Gio build dependencies (it compiles both its Wayland and X11 backends, so
# both sets of headers are needed even though we run under sway).
sudo apt install -y gcc pkg-config \
  libwayland-dev libxkbcommon-dev libxkbcommon-x11-dev \
  libx11-dev libx11-xcb-dev \
  libegl1-mesa-dev libgles2-mesa-dev libffi-dev libxcursor-dev libvulkan-dev
# If libegl1-mesa-dev / libgles2-mesa-dev are missing, use libegl-dev / libgles-dev.

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
./deploy/install.sh   # deps, build, install binary + sway config,
                      # enable console autologin, add kiosk autostart to ~/.profile
sudo reboot
```

On boot the Pi autologins on tty1, and `~/.profile` `exec`s
`sway -c /etc/alarmclock/sway.config`, which rotates the display and launches
the app fullscreen. To update: `git pull && ./deploy/install.sh && sudo reboot`.

## Troubleshooting

- **Stops at a text console (app doesn't appear)**: confirm console autologin is
  on (`sudo raspi-config` → System → Boot → Console Autologin), and that the
  kiosk block is at the end of `~/.profile`. Run `sway -c /etc/alarmclock/sway.config`
  by hand on tty1 to see sway's errors.
- **To exit the kiosk / debug**: SSH in and `pkill sway` (the tty1 shell was
  replaced by sway via `exec`, so it won't drop back to a prompt on its own).
  Comment out the kiosk block in `~/.profile` to disable autostart.
- **`Timeout waiting session to become active` / libseat VT permission errors**:
  sway is being started without an active seat — you're almost certainly running
  it **over SSH**. It must run on the physical console (tty1). Reboot and let the
  autostart launch it there. Check with `loginctl session-status` on tty1
  (`Seat: seat0`, `Active: yes`). If it fails even on tty1, install the seatd
  fallback: `sudo apt install -y seatd && sudo systemctl enable --now seatd`, add
  yourself to `video,render,input,_seatd`, and reboot.
- **No sound**: verify the output with `mpv <some.mp3>`; set the default sink
  with `wpctl`/`raspi-config`. The alarm tone is generated on first run at
  `~/.cache/alarmclock/alarm.wav`.
- **Spotify "device not found"**: activate the "Wekker" device once from the
  Spotify phone app; confirm librespot is running (`pgrep librespot`).
