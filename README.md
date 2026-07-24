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
# Run windowed on a desktop (skips kiosk fullscreen)
ALARMCLOCK_WINDOWED=1 go run ./cmd/alarmclock

# Build
go build ./cmd/alarmclock
```

On the Pi the app starts fullscreen (kiosk) via a systemd unit; see `deploy/`.

## Status

Milestone-based build:

1. ✅ Skeleton + Catppuccin Mocha theme + fullscreen window
2. ✅ Clock home screen (analog + digital + Dutch date, ticking) + action buttons
3. ✅ Three alarms + scheduler + firing/snooze screen
4. ✅ Audio controller + mpv (generated alarm tone, fade-in)
5. ⬜ Internet radio (radio-browser.info)
6. ⬜ Spotify (OAuth + librespot + search/library)
7. ⬜ Kiosk deploy + polish
