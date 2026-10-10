#!/usr/bin/env bash
# Run the alarm clock under a restart loop, with its output on disk.
#
# sway execs this instead of the binary directly, for two reasons:
#
#  - Nothing supervises the app. The kiosk is a login-shell sway session, not a
#    systemd service, so a panic or a fatal startup error would leave the screen
#    blank and every alarm dead until the next reboot.
#  - Every log line the app writes goes to sway's stderr on the tty1 console
#    that the kiosk covers, and is then lost. A missed alarm left no evidence.
#
# Read the log with:  tail -f ~/.cache/alarmclock/app.log

set -u

BIN="${ALARMCLOCK_BIN:-/usr/local/bin/alarmclock}"
LOG_DIR="${ALARMCLOCK_LOG_DIR:-$HOME/.cache/alarmclock}"
LOG="$LOG_DIR/app.log"
MAX_BYTES=$((10 * 1024 * 1024))

mkdir -p "$LOG_DIR"

# Rotate on launch, keeping one previous generation. The app logs a heartbeat
# every minute (~150 KB/day), so this caps the pair at ~20 MB.
if [ -f "$LOG" ] && [ "$(stat -c %s "$LOG" 2>/dev/null || echo 0)" -gt "$MAX_BYTES" ]; then
	mv -f "$LOG" "$LOG.1"
fi

# Match Go's log prefix format so supervisor and app lines read as one stream.
note() { printf '%s supervisor: %s\n' "$(date '+%Y/%m/%d %H:%M:%S')" "$1" >>"$LOG"; }

# Silence whatever a dead app left behind. The app runs mpv (alarm tone,
# radio) and librespot (Spotify) as child processes; when it panics or is
# killed they are orphaned and keep playing — a looping alarm tone nobody can
# stop, or a second "Wekker" on Spotify next to the new one. librespot starts
# at volume 0, so the next one comes back silent.
silence_orphans() {
	pkill -u "$(id -u)" -f 'alarmclock-mpv-' 2>/dev/null && note "killed an orphaned mpv"
	pkill -u "$(id -u)" -x librespot 2>/dev/null && note "killed an orphaned librespot"
	return 0
}

note "starting $BIN"
silence_orphans
while :; do
	started=$(date +%s)
	"$BIN" >>"$LOG" 2>&1
	code=$?
	note "alarmclock exited with status $code after $(($(date +%s) - started))s — restarting in 2s"
	silence_orphans
	sleep 2
done
