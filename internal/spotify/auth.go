package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Authorize runs the one-time PKCE authorization flow: it opens the system
// browser to Spotify's consent page and serves a loopback callback to capture
// the authorization code, which it exchanges for tokens. Tokens are stored via
// the onTokens callback. Blocks until done, cancelled or timed out.
func (c *Client) Authorize(ctx context.Context) error {
	if !c.Configured() {
		return fmt.Errorf("spotify: no client ID configured")
	}
	verifier, err := randString(64)
	if err != nil {
		return err
	}
	state, err := randString(16)
	if err != nil {
		return err
	}

	redirect, err := url.Parse(c.cfg.RedirectURI)
	if err != nil {
		return fmt.Errorf("spotify: bad redirect URI: %w", err)
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURI)
	q.Set("scope", strings.Join(Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", pkceChallenge(verifier))
	authURL := c.accounts + "/authorize?" + q.Encode()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(redirect.Path, func(w http.ResponseWriter, r *http.Request) {
		rq := r.URL.Query()
		if e := rq.Get("error"); e != "" {
			http.Error(w, e, http.StatusBadRequest)
			errCh <- fmt.Errorf("spotify: authorization denied: %s", e)
			return
		}
		if rq.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("spotify: state mismatch")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><body style='font-family:sans-serif;background:#1e1e2e;color:#cdd6f4;text-align:center;padding-top:3rem'><h2>Spotify gekoppeld ✓</h2><p>Je kunt dit venster sluiten.</p></body>"))
		codeCh <- rq.Get("code")
	})

	ln, err := net.Listen("tcp", redirect.Host)
	if err != nil {
		return fmt.Errorf("spotify: cannot listen on %s: %w", redirect.Host, err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	if err := openBrowser(authURL); err != nil {
		log.Printf("spotify: open this URL to authorize:\n%s", authURL)
	}

	select {
	case code := <-codeCh:
		toks, err := c.exchangeCode(ctx, code, verifier)
		if err != nil {
			return err
		}
		c.setTokens(toks)
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("spotify: authorization timed out")
	}
}

func (c *Client) exchangeCode(ctx context.Context, code, verifier string) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.cfg.RedirectURI)
	form.Set("client_id", c.cfg.ClientID)
	form.Set("code_verifier", verifier)
	return c.tokenRequest(ctx, form)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
