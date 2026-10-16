package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"alarmclock/internal/alarm"
	"alarmclock/internal/spotify"
)

func TestMain(m *testing.M) {
	playbackConfirm = 2 * time.Second
	playbackPoll = 20 * time.Millisecond
	revealConfirm = time.Second
	retryInterval = 50 * time.Millisecond
	librespotRecheck = 200 * time.Millisecond
	rampUp = 200 * time.Millisecond
	if os.Getenv("RINGER_LOG") == "" {
		log.SetOutput(io.Discard)
	}
	os.Exit(m.Run())
}

// fakeDevice is Spotify's view of our librespot device, served as the subset
// of the Web API the ringer uses.
type fakeDevice struct {
	mu sync.Mutex
	deviceState
}

type deviceState struct {
	present   bool // on the device list
	active    bool // holds playback
	playing   bool
	progress  int
	volume    int
	context   string
	nexts     int
	audibleAt time.Time // first moment it played with the volume up
}

func newDevice(present bool) *fakeDevice {
	return &fakeDevice{deviceState: deviceState{present: present}}
}

func (d *fakeDevice) set(f func(s *deviceState)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f(&d.deviceState)
}

func (d *fakeDevice) get() deviceState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deviceState
}

func (d *fakeDevice) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer func() {
		if d.playing && d.volume > 0 && d.audibleAt.IsZero() {
			d.audibleAt = time.Now()
		}
		d.mu.Unlock()
	}()
	dev := map[string]any{"id": "dev1", "name": "Wekker", "is_active": d.active, "volume_percent": d.volume}
	q := r.URL.Query()
	switch r.Method + " " + r.URL.Path {
	case "GET /me/player/devices":
		devs := []any{}
		if d.present {
			devs = append(devs, dev)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": devs})
	case "GET /me/player":
		if !d.active {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if d.playing {
			d.progress += 500
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"is_playing": d.playing, "progress_ms": d.progress, "device": dev})
	case "PUT /me/player/play":
		if !d.present {
			http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
			return
		}
		var body struct {
			ContextURI string `json:"context_uri"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.active, d.playing, d.progress, d.context = true, true, 0, body.ContextURI
	case "PUT /me/player/volume":
		if !d.active {
			http.Error(w, `{"error":"no active device"}`, http.StatusNotFound)
			return
		}
		d.volume, _ = strconv.Atoi(q.Get("volume_percent"))
	case "PUT /me/player/pause":
		d.playing = false
	case "PUT /me/player/shuffle":
	case "POST /me/player/next":
		d.progress = 0
		d.nexts++
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

type fakeTone struct {
	mu   sync.Mutex
	on   bool
	rang int
}

func (t *fakeTone) Start(alarm.Alarm) { t.mu.Lock(); t.on = true; t.rang++; t.mu.Unlock() }
func (t *fakeTone) Stop()             { t.mu.Lock(); t.on = false; t.mu.Unlock() }
func (t *fakeTone) state() (on bool, rang int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.on, t.rang
}

type fakeLibrespot struct {
	dev      *fakeDevice
	restarts int
	mu       sync.Mutex
}

// Restart brings the device back, as a fresh librespot does: listed, idle, at
// volume 0.
func (l *fakeLibrespot) Restart() {
	l.mu.Lock()
	l.restarts++
	l.mu.Unlock()
	l.dev.set(func(s *deviceState) { *s = deviceState{present: true} })
}

func newTestRinger(t *testing.T, dev *fakeDevice) (*alarmRinger, *fakeTone, *fakeLibrespot) {
	t.Helper()
	srv := httptest.NewServer(dev)
	t.Cleanup(srv.Close)
	spot := spotify.New(spotify.Config{ClientID: "cid", APIBase: srv.URL},
		spotify.Tokens{AccessToken: "A", RefreshToken: "R", Expiry: time.Now().Add(time.Hour)}, nil)
	tone := &fakeTone{}
	lib := &fakeLibrespot{dev: dev}
	r := newAlarmRinger(tone, spot, lib, "Wekker")
	t.Cleanup(func() {
		r.Stop()
		r.inspect(func() {}) // let the Stop land before the next test
	})
	return r, tone, lib
}

var spotifyAlarm = alarm.Alarm{Enabled: true, Hour: 7, Minute: 0,
	Sound: alarm.Sound{Kind: alarm.SoundSpotify, Ref: "spotify:playlist:wake"}}

// eventually polls cond until it holds or a generous deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// inspect runs f on the ringer's loop goroutine and waits for it.
func (r *alarmRinger) inspect(f func()) {
	done := make(chan struct{})
	r.do(func() { f(); close(done) })
	<-done
}

func (r *alarmRinger) isReady() (ready bool) {
	r.inspect(func() { ready = r.sess != nil && r.sess.ready })
	return ready
}

func (t *fakeTone) isOn() bool { on, _ := t.state(); return on }

// The whole point: a warmed-up alarm plays its playlist inaudibly until it is
// due, then skips to a fresh track and ramps up — without the tone.
func TestWarmUpPlaysSilentlyThenRampsUpWithoutTheTone(t *testing.T) {
	dev := newDevice(true)
	r, tone, _ := newTestRinger(t, dev)

	r.Prepare(spotifyAlarm, time.Now().Add(2*time.Minute))
	eventually(t, "the warm-up is playing", r.isReady)
	if st := dev.get(); st.context != spotifyAlarm.Sound.Ref || !st.playing || st.volume != 0 || !st.audibleAt.IsZero() {
		t.Fatalf("warm-up should play the playlist at volume 0, got %+v", st)
	}

	due := time.Now()
	r.Start(spotifyAlarm)
	eventually(t, "the volume is fully up", func() bool { return dev.get().volume == 100 })
	st := dev.get()
	if st.nexts != 1 {
		t.Fatalf("ringing should skip to a fresh track once, skipped %d", st.nexts)
	}
	if st.audibleAt.Before(due) {
		t.Fatalf("music became audible %s before the alarm was due", due.Sub(st.audibleAt))
	}
	if _, rang := tone.state(); rang != 0 {
		t.Fatalf("a warmed-up alarm should not ring the tone, rang %d times", rang)
	}
}

// Snoozing drops the volume to 0 but keeps playing; ringing again ramps up.
func TestSnoozeKeepsPlayingSilently(t *testing.T) {
	dev := newDevice(true)
	r, tone, _ := newTestRinger(t, dev)

	r.Prepare(spotifyAlarm, time.Now())
	eventually(t, "the warm-up is playing", r.isReady)
	r.Start(spotifyAlarm)
	eventually(t, "the volume is up", func() bool { return dev.get().volume == 100 })

	r.Snooze()
	eventually(t, "the volume is down", func() bool { return dev.get().volume == 0 })
	if !dev.get().playing {
		t.Fatal("snooze should keep the playlist playing")
	}

	r.Start(spotifyAlarm)
	eventually(t, "the volume is up again", func() bool { return dev.get().volume == 100 })
	if st := dev.get(); st.nexts != 2 {
		t.Fatalf("ringing after a snooze should skip to a fresh track, skips=%d", st.nexts)
	}
	if _, rang := tone.state(); rang != 0 {
		t.Fatalf("tone rang %d times", rang)
	}
}

// Stop and a cancelled warm-up both leave the device paused at volume 0.
func TestStopAndCancelLeaveTheDeviceQuiet(t *testing.T) {
	for name, end := range map[string]func(r *alarmRinger){
		"stop":           func(r *alarmRinger) { r.Stop() },
		"cancel-prepare": func(r *alarmRinger) { r.CancelPrepare() },
	} {
		t.Run(name, func(t *testing.T) {
			dev := newDevice(true)
			r, _, _ := newTestRinger(t, dev)
			r.Prepare(spotifyAlarm, time.Now())
			eventually(t, "the warm-up is playing", r.isReady)

			end(r)
			eventually(t, "the device is paused", func() bool { return !dev.get().playing })
			if v := dev.get().volume; v != 0 {
				t.Fatalf("volume left at %d", v)
			}
		})
	}
}

// A ringing alarm survives CancelPrepare: that is only for warm-ups.
func TestCancelPrepareLeavesARingingAlarm(t *testing.T) {
	dev := newDevice(true)
	r, _, _ := newTestRinger(t, dev)
	r.Prepare(spotifyAlarm, time.Now())
	eventually(t, "the warm-up is playing", r.isReady)
	r.Start(spotifyAlarm)
	r.CancelPrepare()
	eventually(t, "the volume is up", func() bool { return dev.get().volume == 100 })
}

// If Spotify cannot play when the alarm is due, the tone rings — and stops
// once the session has recovered (here by restarting librespot).
func TestToneCoversAFailedWarmUpUntilMusicPlays(t *testing.T) {
	dev := newDevice(false) // librespot dropped off overnight
	r, tone, lib := newTestRinger(t, dev)
	lib.mu.Lock() // keep librespot down until the alarm is due
	r.Prepare(spotifyAlarm, time.Now())
	time.Sleep(100 * time.Millisecond)

	r.Start(spotifyAlarm)
	eventually(t, "the tone rings while Spotify is not playing", tone.isOn)
	lib.mu.Unlock()

	eventually(t, "the music is up", func() bool { return dev.get().volume == 100 })
	if tone.isOn() {
		t.Fatal("the tone should stop once music plays")
	}
}

// A tone alarm never touches Spotify's playback, and takes over from a
// warm-up for another alarm.
func TestToneAlarmEndsAWarmUp(t *testing.T) {
	dev := newDevice(true)
	r, tone, _ := newTestRinger(t, dev)
	r.Prepare(spotifyAlarm, time.Now())
	eventually(t, "the warm-up is playing", r.isReady)

	r.Start(alarm.Alarm{Enabled: true, Hour: 7})
	eventually(t, "the tone alarm rings", tone.isOn)
	eventually(t, "the warm-up is paused", func() bool { return !dev.get().playing })
}

// Someone played to the Wekker from their phone at 60% and paused. The idle
// device refuses a volume change, so the warm-up must not play it as it is:
// a fresh librespot comes back at 0.
func TestWarmUpNeverPlaysADeviceLeftUp(t *testing.T) {
	dev := newDevice(true)
	dev.set(func(s *deviceState) { s.volume = 60 })
	r, _, lib := newTestRinger(t, dev)

	r.Prepare(spotifyAlarm, time.Now().Add(2*time.Minute))
	eventually(t, "the warm-up is playing", r.isReady)
	if st := dev.get(); !st.audibleAt.IsZero() || st.volume != 0 {
		t.Fatalf("warm-up played audibly: %+v", st)
	}
	lib.mu.Lock()
	restarts := lib.restarts
	lib.mu.Unlock()
	if restarts != 1 {
		t.Fatalf("librespot restarts = %d, want 1", restarts)
	}
}

// The silent-time backstop counts from the snooze, not from the last time the
// session happened to wake: a long ring followed by a snooze keeps its music.
func TestLongRingThenSnoozeKeepsTheSession(t *testing.T) {
	defer func(d time.Duration) { silentMax = d }(silentMax)
	silentMax = 500 * time.Millisecond

	dev := newDevice(true)
	r, tone, _ := newTestRinger(t, dev)
	r.Start(spotifyAlarm)
	eventually(t, "the music is up", func() bool { return dev.get().volume == 100 })
	time.Sleep(2 * silentMax) // ringing with music, longer than silentMax

	r.Snooze()
	time.Sleep(silentMax / 2)
	r.Start(spotifyAlarm)
	eventually(t, "the music is up again", func() bool { return dev.get().volume == 100 && dev.get().nexts == 2 })
	if _, rang := tone.state(); rang != 1 { // once, before the first start confirmed
		t.Fatalf("tone rang %d times, want 1", rang)
	}
}
