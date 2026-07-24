# Deploying on the Raspberry Pi 5

The app is built **natively on the Pi** (Gio needs cgo on Linux, which makes
cross-compiling from macOS painful). It runs fullscreen as a systemd service
inside [`cage`](https://github.com/cage-kiosk/cage), a single-application
Wayland compositor — no desktop required.

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
sudo apt install -y cage mpv
```

**librespot** (Spotify Connect device) isn't in apt; install a binary or build it:

```sh
# via cargo (rustup), or download a prebuilt aarch64 binary to /usr/local/bin
cargo install librespot
```

## 2. Display orientation

The Touch Display 2 is 720×1280 native; the app expects **landscape 1280×720**.
Rotate at the compositor/KMS level (the app just uses the resulting resolution).
For a DSI panel, add to `/boot/firmware/config.txt`, e.g.:

```
dtoverlay=vc4-kms-v3d
video=DSI-1:720x1280@60,rotate=270
```

Adjust `rotate=90|270` for the orientation you want, then reboot. (You can test
rotation live under Wayland with `wlr-randr --output DSI-1 --transform 270`.)

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

- **Black screen / cage won't start**: ensure the service runs on the seat that
  owns the display (`TTYPath`), and that the user is in the `video`/`render`
  groups. Check `journalctl -u alarmclock`.
- **No sound**: verify the output with `mpv <some.mp3>`; set the default sink
  with `wpctl`/`raspi-config`. The alarm tone is generated on first run at
  `~/.cache/alarmclock/alarm.wav`.
- **Spotify "device not found"**: activate the "Wekker" device once from the
  Spotify phone app; confirm librespot is running (`pgrep librespot`).
