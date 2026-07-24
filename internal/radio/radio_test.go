package radio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(srv *httptest.Server) *Client {
	return &Client{http: srv.Client(), base: srv.URL, ua: "test"}
}

func TestSearchParsesStations(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent header")
		}
		_, _ = w.Write([]byte(`[
			{"stationuuid":"1","name":"NPO Radio 2","url":"http://x/stream","url_resolved":"http://x/resolved","tags":"pop","country":"The Netherlands","codec":"MP3","bitrate":192}
		]`))
	}))
	defer srv.Close()

	st, err := testClient(srv).Search(context.Background(), "npo radio", 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 {
		t.Fatalf("got %d stations, want 1", len(st))
	}
	if st[0].Name != "NPO Radio 2" || st[0].Bitrate != 192 {
		t.Errorf("unexpected station: %+v", st[0])
	}
	if st[0].StreamURL() != "http://x/resolved" {
		t.Errorf("StreamURL = %q, want resolved URL", st[0].StreamURL())
	}
	if !strings.Contains(gotPath, "name=npo+radio") || !strings.Contains(gotPath, "limit=25") {
		t.Errorf("unexpected request path: %s", gotPath)
	}
}

func TestStreamURLFallsBackToURL(t *testing.T) {
	s := Station{URL: "http://x/only"}
	if s.StreamURL() != "http://x/only" {
		t.Errorf("StreamURL = %q, want fallback URL", s.StreamURL())
	}
}

func TestTopHitsTopclickEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := testClient(srv)
	c.http.Timeout = 5 * time.Second
	if _, err := c.Top(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/json/stations/topclick/10" {
		t.Errorf("path = %q, want /json/stations/topclick/10", gotPath)
	}
}
