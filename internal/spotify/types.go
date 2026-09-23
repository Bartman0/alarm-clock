// Package spotify is a small client for the Spotify Web API: OAuth (PKCE),
// search, library browsing, device listing and playback control. It controls a
// librespot instance running on the Pi as a Spotify Connect device.
package spotify

import "time"

// Tokens holds the OAuth token set persisted between runs.
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// Valid reports whether the access token is present and not (nearly) expired.
func (t Tokens) Valid() bool {
	return t.AccessToken != "" && time.Now().Before(t.Expiry.Add(-30*time.Second))
}

// Artist is a Spotify artist.
type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Track is a Spotify track.
type Track struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Playlist is a Spotify playlist.
type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// PlaybackState is what Spotify reports about current playback. ProgressMS is
// the position in the current track: it advancing between two reads is the
// only real evidence that audio is being rendered, since is_playing can be
// true while the device produces nothing.
type PlaybackState struct {
	IsPlaying  bool   `json:"is_playing"`
	ProgressMS int    `json:"progress_ms"`
	Device     Device `json:"device"`
}

// Device is a Spotify Connect playback device.
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}
