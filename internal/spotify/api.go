package spotify

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// SearchArtists searches for artists by name.
//
// We deliberately send no "limit": some Spotify apps/tokens (development-mode)
// reject an explicit limit with 400 "Invalid limit", so we take the default.
// Spotify's search can also reject '+'-encoded spaces, so we use %20.
func (c *Client) SearchArtists(ctx context.Context, query string) ([]Artist, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("type", "artist")
	raw := strings.ReplaceAll(q.Encode(), "+", "%20")
	var body struct {
		Artists struct {
			Items []Artist `json:"items"`
		} `json:"artists"`
	}
	if err := c.apiGet(ctx, "/search?"+raw, &body); err != nil {
		return nil, err
	}
	return body.Artists.Items, nil
}

// ArtistTopTracks returns an artist's top tracks in the client's market.
func (c *Client) ArtistTopTracks(ctx context.Context, artistID string) ([]Track, error) {
	var body struct {
		Tracks []Track `json:"tracks"`
	}
	path := fmt.Sprintf("/artists/%s/top-tracks?market=%s", url.PathEscape(artistID), c.market)
	if err := c.apiGet(ctx, path, &body); err != nil {
		return nil, err
	}
	return body.Tracks, nil
}

// SavedPlaylists returns the current user's playlists (Spotify's default page
// size; see the note on SearchArtists about omitting an explicit limit).
func (c *Client) SavedPlaylists(ctx context.Context) ([]Playlist, error) {
	var body struct {
		Items []Playlist `json:"items"`
	}
	if err := c.apiGet(ctx, "/me/playlists", &body); err != nil {
		return nil, err
	}
	return body.Items, nil
}

// FollowedArtists returns the artists the user follows.
func (c *Client) FollowedArtists(ctx context.Context) ([]Artist, error) {
	var body struct {
		Artists struct {
			Items []Artist `json:"items"`
		} `json:"artists"`
	}
	if err := c.apiGet(ctx, "/me/following?type=artist", &body); err != nil {
		return nil, err
	}
	return body.Artists.Items, nil
}

// Devices lists the user's available Spotify Connect devices.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var body struct {
		Devices []Device `json:"devices"`
	}
	if err := c.apiGet(ctx, "/me/player/devices", &body); err != nil {
		return nil, err
	}
	return body.Devices, nil
}

// DeviceIDByName returns the id of the first device whose name matches, and
// whether it was found. Used to locate our librespot instance.
func (c *Client) DeviceIDByName(ctx context.Context, name string) (string, bool, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return "", false, err
	}
	for _, d := range devs {
		if d.Name == name {
			return d.ID, true, nil
		}
	}
	return "", false, nil
}

// Play starts playback on the given device. A contextURI (album/playlist/
// artist) or a set of track URIs may be supplied.
func (c *Client) Play(ctx context.Context, deviceID, contextURI string, uris []string) error {
	body := map[string]any{}
	if contextURI != "" {
		body["context_uri"] = contextURI
	}
	if len(uris) > 0 {
		body["uris"] = uris
	}
	path := "/me/player/play"
	if deviceID != "" {
		path += "?device_id=" + url.QueryEscape(deviceID)
	}
	return c.apiSend(ctx, "PUT", path, body)
}

// Pause pauses playback.
func (c *Client) Pause(ctx context.Context) error {
	return c.apiSend(ctx, "PUT", "/me/player/pause", nil)
}

// Shuffle sets the shuffle state on the given device.
func (c *Client) Shuffle(ctx context.Context, deviceID string, state bool) error {
	path := fmt.Sprintf("/me/player/shuffle?state=%t", state)
	if deviceID != "" {
		path += "&device_id=" + url.QueryEscape(deviceID)
	}
	return c.apiSend(ctx, "PUT", path, nil)
}

// Next skips to the next track on the given device.
func (c *Client) Next(ctx context.Context, deviceID string) error {
	path := "/me/player/next"
	if deviceID != "" {
		path += "?device_id=" + url.QueryEscape(deviceID)
	}
	return c.apiSend(ctx, "POST", path, nil)
}

// Transfer moves playback to the given device (and starts playing).
func (c *Client) Transfer(ctx context.Context, deviceID string) error {
	return c.apiSend(ctx, "PUT", "/me/player", map[string]any{
		"device_ids": []string{deviceID},
		"play":       true,
	})
}

// PlayOnDevice resolves the named Connect device and starts playback of a
// context (playlist/artist/album URI) or track URIs on it.
func (c *Client) PlayOnDevice(ctx context.Context, deviceName, contextURI string, uris []string) error {
	id, ok, err := c.DeviceIDByName(ctx, deviceName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("spotify: device %q not found — open Spotify on your phone and select it once", deviceName)
	}
	return c.Play(ctx, id, contextURI, uris)
}
