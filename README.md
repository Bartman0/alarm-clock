# Alarm Clock

A touchscreen alarm clock for the **Raspberry Pi 5** with the official
**Touch Display 2** (1280×720, landscape). Built in **Go** with the
[**Gio**](https://gioui.org) UI toolkit.

## Features (target)

- Analog **and** digital clock, 24-hour time, Dutch weekday + `DD-MM-YYYY` date.
- **Three alarms**, each with a full-week / workweek / weekend rhythm.
- On fire: play a built-in **alarm sound** or a **Spotify** song; **5-minute snooze**.
- During normal use: play **internet radio** (radio-browser.info) or **Spotify**,
  each behind its own clear button.
- **Catppuccin Mocha** theme, modern look, large fonts.

## Architecture

Single Go binary (Gio UI on the main thread) with background goroutines for the
clock tick, alarm scheduler and audio control. Audio is played by two
subprocesses so nothing is decoded in-process:

- **librespot** — turns the Pi into a Spotify Connect device (Premium required),
  controlled via the Spotify Web API.
- **mpv** — plays internet-radio stream URLs and the alarm sounds (JSON IPC).

## Development

```sh
make run     # run windowed on a desktop (skips kiosk fullscreen)
make build   # build the binary
make test    # run the test suite
```

Without `mpv`/`librespot`/network (e.g. on a dev Mac) the app runs fine and
degrades gracefully: alarms log instead of sounding, and Spotify shows a
"not configured" state.

## Deployment (Raspberry Pi)

The app builds natively on the Pi and runs fullscreen via systemd inside the
`cage` Wayland kiosk compositor. See **[`deploy/README.md`](deploy/README.md)**
for the full setup; in short:

```sh
./deploy/install.sh            # build, install binary + service, enable on boot
sudo systemctl start alarmclock
```

## Status

Milestone-based build:

1. ✅ Skeleton + Catppuccin Mocha theme + fullscreen window
2. ✅ Clock home screen (analog + digital + Dutch date, ticking) + action buttons
3. ✅ Three alarms + scheduler + firing/snooze screen
4. ✅ Audio controller + mpv (generated alarm tone, fade-in)
5. ✅ Internet radio (radio-browser.info) — browse/search, stream via mpv
6. ✅ Spotify (OAuth PKCE + librespot + search/library + alarm playlist)
7. ✅ Kiosk deploy (cage + systemd) + build/install tooling
