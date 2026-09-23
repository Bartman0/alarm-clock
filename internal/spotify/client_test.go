package spotify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApiRefreshesExpiredToken(t *testing.T) {
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "R" {
			t.Errorf("refresh_token = %q", r.FormValue("refresh_token"))
		}
		_, _ = w.Write([]byte(`{"access_token":"NEW","expires_in":3600}`))
	}))
	defer accounts.Close()

	var gotAuth string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"artists":{"items":[]}}`))
	}))
	defer api.Close()

	var saved Tokens
	c := New(Config{ClientID: "cid"}, Tokens{RefreshToken: "R", Expiry: time.Now().Add(-time.Hour)}, func(tk Tokens) { saved = tk })
	c.accounts = accounts.URL
	c.api = api.URL

	if _, err := c.SearchArtists(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer NEW" {
		t.Errorf("Authorization = %q, want Bearer NEW", gotAuth)
	}
	if saved.AccessToken != "NEW" {
		t.Errorf("access token not persisted: %+v", saved)
	}
	if saved.RefreshToken != "R" {
		t.Errorf("refresh token not preserved across refresh: %+v", saved)
	}
}

func TestSearchArtistsParses(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "artist" {
			t.Errorf("type = %q", got)
		}
		_, _ = w.Write([]byte(`{"artists":{"items":[{"id":"1","name":"Radiohead","uri":"spotify:artist:1"}]}}`))
	}))
	defer api.Close()

	c := validClient(api.URL)
	got, err := c.SearchArtists(context.Background(), "radiohead")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Radiohead" || got[0].URI != "spotify:artist:1" {
		t.Errorf("unexpected artists: %+v", got)
	}
}

func TestPlaySendsDeviceAndContext(t *testing.T) {
	var gotPath, gotBody string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()

	c := validClient(api.URL)
	if err := c.Play(context.Background(), "dev1", "spotify:playlist:42", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/me/player/play") || !strings.Contains(gotPath, "device_id=dev1") {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"context_uri":"spotify:playlist:42"`) {
		t.Errorf("body = %q", gotBody)
	}
}

func TestNotAuthorizedError(t *testing.T) {
	c := New(Config{ClientID: "cid"}, Tokens{}, nil)
	if _, err := c.Devices(context.Background()); err == nil {
		t.Fatal("expected error when not authorized")
	}
}

// validClient returns a client with a live (non-expired) access token pointed
// at the given API base.
func validClient(apiURL string) *Client {
	c := New(Config{ClientID: "cid"}, Tokens{AccessToken: "A", RefreshToken: "R", Expiry: time.Now().Add(time.Hour)}, nil)
	c.api = apiURL
	return c
}

func TestPlaybackStateReportsProgress(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/player" {
			t.Errorf("path = %q, want /me/player", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"is_playing":true,"progress_ms":4321,"device":{"id":"D1","name":"Wekker","is_active":true}}`))
	}))
	defer api.Close()

	c := New(Config{ClientID: "cid"}, Tokens{AccessToken: "A", Expiry: time.Now().Add(time.Hour)}, nil)
	c.api = api.URL

	st, ok, err := c.PlaybackState(context.Background())
	if err != nil || !ok {
		t.Fatalf("PlaybackState: ok=%v err=%v", ok, err)
	}
	if !st.IsPlaying || st.ProgressMS != 4321 || st.Device.ID != "D1" {
		t.Fatalf("got %+v", st)
	}
}

// Spotify answers 204 with an empty body when nothing is playing anywhere.
// That is an answer, not a failure — and it is the state the alarm must be
// able to tell apart from "playing", since it decides whether the tone stops.
func TestPlaybackStateHandlesNoContent(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()

	c := New(Config{ClientID: "cid"}, Tokens{AccessToken: "A", Expiry: time.Now().Add(time.Hour)}, nil)
	c.api = api.URL

	st, ok, err := c.PlaybackState(context.Background())
	if err != nil {
		t.Fatalf("204 should not be an error: %v", err)
	}
	if ok || st.IsPlaying {
		t.Fatalf("204 should report nothing playing, got ok=%v %+v", ok, st)
	}
}

// Endpoints that must return data still treat an empty response as an error.
func TestApiGetTreatsNoContentAsError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()

	c := New(Config{ClientID: "cid"}, Tokens{AccessToken: "A", Expiry: time.Now().Add(time.Hour)}, nil)
	c.api = api.URL

	if _, err := c.Devices(context.Background()); err == nil {
		t.Fatal("Devices should fail when the API returns no content")
	}
}
