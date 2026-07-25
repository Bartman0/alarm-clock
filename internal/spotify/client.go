package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultRedirectURI is the loopback callback used during the first-run auth.
const DefaultRedirectURI = "http://127.0.0.1:8888/callback"

// Scopes requested: playback control plus read access to the user's library.
var Scopes = []string{
	"user-read-playback-state",
	"user-modify-playback-state",
	"playlist-read-private",
	"user-follow-read",
	"user-library-read",
}

// Config configures the OAuth client.
type Config struct {
	ClientID    string
	RedirectURI string
}

// Client talks to the Spotify Web API and manages OAuth tokens.
type Client struct {
	cfg      Config
	http     *http.Client
	accounts string // https://accounts.spotify.com
	api      string // https://api.spotify.com/v1
	market   string // country code for track relinking, e.g. "NL"
	onTokens func(Tokens)
	mu       sync.Mutex
	tokens   Tokens
}

// New returns a client seeded with any stored tokens. onTokens is called
// whenever the token set changes so the caller can persist it.
func New(cfg Config, tokens Tokens, onTokens func(Tokens)) *Client {
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = DefaultRedirectURI
	}
	return &Client{
		cfg:      cfg,
		http:     &http.Client{Timeout: 15 * time.Second},
		accounts: "https://accounts.spotify.com",
		api:      "https://api.spotify.com/v1",
		market:   "NL",
		onTokens: onTokens,
		tokens:   tokens,
	}
}

// Configured reports whether a client ID is set.
func (c *Client) Configured() bool { return c.cfg.ClientID != "" }

// Authorized reports whether we hold a refresh token (i.e. first-run auth is
// complete).
func (c *Client) Authorized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens.RefreshToken != ""
}

func (c *Client) setTokens(t Tokens) {
	c.mu.Lock()
	// Spotify may omit a new refresh token on refresh; keep the old one.
	if t.RefreshToken == "" {
		t.RefreshToken = c.tokens.RefreshToken
	}
	c.tokens = t
	cb := c.onTokens
	c.mu.Unlock()
	if cb != nil {
		cb(t)
	}
}

// accessToken returns a valid access token, refreshing if necessary.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok := c.tokens
	c.mu.Unlock()

	if tok.Valid() {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("spotify: not authorized")
	}
	refreshed, err := c.refresh(ctx, tok.RefreshToken)
	if err != nil {
		return "", err
	}
	c.setTokens(refreshed)
	return refreshed.AccessToken, nil
}

// refresh exchanges a refresh token for a new access token.
func (c *Client) refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", c.cfg.ClientID)
	return c.tokenRequest(ctx, form)
}

// tokenRequest posts to the token endpoint and parses the token response.
func (c *Client) tokenRequest(ctx context.Context, form url.Values) (Tokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.accounts+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("spotify token: %s", resp.Status)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Tokens{}, err
	}
	return Tokens{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}, nil
}

// apiGet performs an authorized GET and decodes JSON into out.
func (c *Client) apiGet(ctx context.Context, path string, out any) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify GET %s: %s: %s", path, resp.Status, errBody(resp))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// errBody reads a short prefix of an error response body for diagnostics.
func errBody(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return strings.TrimSpace(string(b))
}

// apiSend performs an authorized PUT/POST with an optional JSON body. 2xx and
// 204 are treated as success.
func (c *Client) apiSend(ctx context.Context, method, path string, body any) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var reader *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("spotify %s %s: %s: %s", method, path, resp.Status, errBody(resp))
	}
	return nil
}
