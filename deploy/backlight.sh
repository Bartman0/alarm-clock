#!/usr/bin/env bash
# Set every backlight device to <percent> of its maximum brightness.
# Used by swayidle to dim the screen when idle and restore it on touch.
# Usage: backlight.sh <0-100>
pct="${1:-100}"
for bl in /sys/class/backlight/*; do
	[ -r "$bl/max_brightness" ] || continue
	max=$(cat "$bl/max_brightness")
	val=$((max * pct / 100))
	# Never fully off for a non-zero request (keep the panel visibly lit).
	if [ "$pct" -gt 0 ] && [ "$val" -lt 1 ]; then
		val=1
	fi
	echo "$val" >"$bl/brightness" 2>/dev/null || true
done
