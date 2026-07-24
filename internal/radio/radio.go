// Package radio is a small client for the radio-browser.info public API used
// to browse and search internet radio stations.
package radio

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Station is a radio-browser station (only the fields we use).
type Station struct {
	UUID     string `json:"stationuuid"`
	Name     string `json:"name"`
	Resolved string `json:"url_resolved"`
	URL      string `json:"url"`
	Tags     string `json:"tags"`
	Country  string `json:"country"`
	Codec    string `json:"codec"`
	Bitrate  int    `json:"bitrate"`
}

// StreamURL returns the best playable URL for the station.
func (s Station) StreamURL() string {
	if s.Resolved != "" {
		return s.Resolved
	}
	return s.URL
}

// fallbackServers are used when DNS discovery of the mirror pool fails.
var fallbackServers = []string{
	"https://de1.api.radio-browser.info",
	"https://nl1.api.radio-browser.info",
	"https://at1.api.radio-browser.info",
	"https://fi1.api.radio-browser.info",
}

// Client talks to one radio-browser mirror.
type Client struct {
	http *http.Client
	base string
	ua   string
}

// NewClient picks a mirror (via DNS discovery, falling back to a static list)
// and returns a ready client. It performs a DNS lookup, so call it off the UI
// goroutine.
func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 12 * time.Second},
		base: pickServer(),
		ua:   "alarmclock/0.1 (Raspberry Pi alarm clock; +https://xomnia.com)",
	}
}

// pickServer resolves the radio-browser mirror pool and returns a base URL.
func pickServer() string {
	ips, err := net.LookupIP("all.api.radio-browser.info")
	if err == nil {
		for _, ip := range ips {
			if names, err := net.LookupAddr(ip.String()); err == nil && len(names) > 0 {
				host := strings.TrimSuffix(names[0], ".")
				if host != "" {
					return "https://" + host
				}
			}
		}
	}
	return fallbackServers[0]
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("radio-browser: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Top returns the most-clicked stations, for the initial browse view.
func (c *Client) Top(ctx context.Context, limit int) ([]Station, error) {
	var st []Station
	err := c.get(ctx, fmt.Sprintf("/json/stations/topclick/%d", limit), &st)
	return st, err
}

// Search returns stations matching name, ordered by popularity.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Station, error) {
	q := url.Values{}
	q.Set("name", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("hidebroken", "true")
	q.Set("order", "clickcount")
	q.Set("reverse", "true")
	var st []Station
	err := c.get(ctx, "/json/stations/search?"+q.Encode(), &st)
	return st, err
}
