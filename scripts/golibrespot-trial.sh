#!/usr/bin/env bash
# Try go-librespot's local API on the Pi, next to the app's own librespot.
# It replays what a Spotify alarm does: warm up silently, then skip and ramp
# up. See `usage` below; API overrides the daemon's address.
set -euo pipefail

API=${API:-http://localhost:3678}

usage() {
	cat >&2 <<'EOF'
usage: golibrespot-trial.sh <command> [playlist-uri]
  ready           is the daemon bootstrapped and ready for /player/play?
  status          playing/paused/stopped, volume and track position
  warmup <uri>    shuffle on, play <uri> at volume 0, show the position moving
  ring            skip to a fresh track and ramp the volume 0 -> 100 over 20s
  quiet           pause and return the volume to 0
  events          stream the /events websocket (needs websocat)
EOF
	exit 2
}

get() { curl -fsS "$API$1"; echo; }
post() {
	local body=${2:-'{}'}
	curl -fsS -X POST "$API$1" -H 'Content-Type: application/json' -d "$body"
}

# summary prints the parts of /status an alarm cares about.
summary() {
	curl -fsS "$API/status" | python3 -c '
import json, sys
raw = sys.stdin.read()
if not raw.strip():
    print("no active session"); sys.exit()
s = json.loads(raw)
t = s.get("track") or {}
state = "stopped" if s["stopped"] else "paused" if s["paused"] else "buffering" if s["buffering"] else "playing"
print("%s  volume=%s/%s  position=%sms  %s" % (state, s["volume"], s["volume_steps"], t.get("position", "-"), t.get("name", "")))
'
}

case "${1:-}" in
ready) get / ;;
status) summary ;;
warmup)
	uri=${2:-}
	[ -n "$uri" ] || usage
	post /player/volume '{"volume":0}' || echo "(volume not set: no session yet; initial_volume is 0 anyway)"
	post /player/shuffle_context '{"shuffle_context":true}'
	post /player/play "{\"uri\":\"$uri\"}"
	for _ in 1 2 3 4 5; do
		sleep 1
		summary
	done
	;;
ring)
	post /player/next
	sleep 1
	summary
	for v in 10 20 30 40 50 60 70 80 90 100; do
		sleep 2
		post /player/volume "{\"volume\":$v}"
	done
	summary
	;;
quiet)
	post /player/pause
	post /player/volume '{"volume":0}'
	summary
	;;
events) websocat "${API/http/ws}/events" ;;
*) usage ;;
esac
