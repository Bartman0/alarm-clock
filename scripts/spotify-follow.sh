#!/usr/bin/env bash
# Follow (add to library) a Spotify playlist via the Web API, then print
# /me/playlists — to test whether an API follow shows up in the library.
#
# Usage: scripts/spotify-follow.sh <playlist url | spotify:playlist:ID | ID>
#
# Following is a write action needing the playlist-modify-* scope; if the
# token lacks it you'll get HTTP 403 (re-authorize the app to grant it).
set -euo pipefail

if [ $# -lt 1 ]; then
	echo "usage: $0 <playlist url | spotify:playlist:ID | ID>" >&2
	exit 1
fi

CONFIG="$HOME/.config/alarmclock/config.json"

# Extract the playlist ID from a URL, URI, or bare ID.
id="$1"
id="${id##*playlist[:/]}" # drop everything up to 'playlist:' or 'playlist/'
id="${id%%[?/]*}"         # drop any trailing ?si=... or /path

get() { python3 -c "import json;print(json.load(open('$CONFIG'))$1)"; }

CLIENT_ID="${ALARMCLOCK_SPOTIFY_CLIENT_ID:-$(get "['spotify']['client_id']")}"
REFRESH=$(get "['spotify']['tokens']['refresh_token']")

# Refresh the access token (PKCE refresh, no client secret needed).
TOKEN=$(curl -s -X POST https://accounts.spotify.com/api/token \
	-d grant_type=refresh_token -d "refresh_token=$REFRESH" -d "client_id=$CLIENT_ID" |
	python3 -c "import sys,json;print(json.load(sys.stdin).get('access_token',''))")

if [ -z "$TOKEN" ]; then
	echo "could not refresh access token (check client_id / refresh_token in $CONFIG)" >&2
	exit 1
fi

echo "== Following playlist $id =="
code=$(curl -s -o /tmp/sp_follow.txt -w "%{http_code}" -X PUT \
	-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
	--data '{"public":false}' \
	"https://api.spotify.com/v1/playlists/$id/followers")
echo "HTTP $code"
if [ -s /tmp/sp_follow.txt ]; then
	echo "response: $(cat /tmp/sp_follow.txt)"
fi

echo "== /me/playlists now =="
curl -s -H "Authorization: Bearer $TOKEN" "https://api.spotify.com/v1/me/playlists" |
	python3 -c "import sys,json;d=json.load(sys.stdin);print('total:',d.get('total'));[print(' -',i['name']) for i in d.get('items',[])]"
