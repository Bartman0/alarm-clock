// Package librespot supervises a librespot process so the Raspberry Pi appears
// as a Spotify Connect device. The user activates the device once from their
// Spotify app (Zeroconf); credentials are cached so it re-registers on boot.
// Playback is then driven via the Spotify Web API (see package spotify).
package librespot

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// autoRestartCooldown rate-limits the watchdog below, so a librespot that
// fails the same way immediately after starting cannot be respawned in a hot
// loop.
const autoRestartCooldown = 2 * time.Minute

// unrecoverableLogLines are librespot messages after which the process can no
// longer play anything, however healthy it otherwise looks.
//
// librespot 0.8.0 recovers its session after a dropped websocket but not its
// audio key channel: it reconnects and re-authenticates cleanly, and then
// fails to decrypt every track it is asked to play ("Unable to load key" →
// "end of stream" → "Skipping to next track"). Nothing short of a restart
// clears it, so we watch its own output for the moment it happens.
//
// Matching log text is brittle across librespot versions; keep this list to
// the one unambiguous signal and re-check it on upgrade.
var unrecoverableLogLines = []string{
	"Audio key response timeout",
}

// unrecoverable reports whether a librespot log line means the process can no
// longer play and must be restarted.
func unrecoverable(line string) bool {
	for _, pattern := range unrecoverableLogLines {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

// Supervisor starts librespot and restarts it if it exits.
type Supervisor struct {
	name     string
	cacheDir string

	mu              sync.Mutex
	cmd             *exec.Cmd
	running         bool
	stop            chan struct{}
	lastAutoRestart time.Time
}

// New returns a supervisor that advertises the given Connect device name.
func New(deviceName string) *Supervisor {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return &Supervisor{
		name:     deviceName,
		cacheDir: filepath.Join(dir, "alarmclock", "librespot"),
	}
}

// Available reports whether the librespot binary is installed.
func (s *Supervisor) Available() bool {
	_, err := exec.LookPath("librespot")
	return err == nil
}

// Start launches librespot and keeps it running until Stop. It is a no-op if
// librespot isn't installed or is already running.
func (s *Supervisor) Start() {
	if !s.Available() {
		log.Printf("librespot: binary not found; Spotify playback device unavailable")
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.mu.Unlock()

	_ = os.MkdirAll(s.cacheDir, 0o755)
	go s.loop()
}

func (s *Supervisor) loop() {
	for {
		select {
		case <-s.stop:
			return
		default:
		}

		cmd := exec.Command("librespot",
			"--name", s.name,
			"--bitrate", "320",
			"--cache", s.cacheDir,
			"--disable-audio-cache",
			// Start at full Connect volume; the USB speaker/PipeWire sink set
			// the actual level, so a low default just makes it seem quiet.
			"--initial-volume", "100",
		)
		// Forward librespot's own output into our log. When an alarm plays
		// nothing, librespot is usually the only thing that knows why (audio
		// backend errors, track load failures, session drops) — and until we
		// captured this it went to /dev/null.
		stdout, errOut := cmd.StdoutPipe()
		stderr, errErr := cmd.StderrPipe()

		s.mu.Lock()
		s.cmd = cmd
		s.mu.Unlock()

		if err := cmd.Start(); err != nil {
			log.Printf("librespot: start failed: %v", err)
		} else {
			var pipes sync.WaitGroup
			for _, pipe := range []struct {
				r   io.ReadCloser
				err error
			}{{stdout, errOut}, {stderr, errErr}} {
				if pipe.err != nil || pipe.r == nil {
					continue
				}
				pipes.Add(1)
				go func(r io.ReadCloser) {
					defer pipes.Done()
					s.relay(r)
				}(pipe.r)
			}
			err := cmd.Wait()
			pipes.Wait() // drain what librespot said on its way out
			if err != nil {
				log.Printf("librespot: exited: %v", err)
			} else {
				log.Printf("librespot: exited cleanly")
			}
		}

		// Back off before restarting, unless we're stopping.
		select {
		case <-s.stop:
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// relay copies a librespot pipe into the app log, one line at a time, and
// restarts librespot when a line says it can no longer play. Restarting on its
// own diagnosis is what keeps an idle-broken process from being discovered by
// an alarm at the worst possible moment.
func (s *Supervisor) relay(r io.ReadCloser) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		log.Printf("librespot: %s", line)
		if !unrecoverable(line) {
			continue
		}
		if !s.claimAutoRestart(time.Now()) {
			log.Printf("librespot: still unable to play, but it was already restarted within %s; leaving it alone", autoRestartCooldown)
			continue
		}
		log.Printf("librespot: that failure cannot be recovered in-process; restarting")
		s.Restart()
	}
}

// claimAutoRestart reports whether the watchdog may restart librespot now,
// recording the attempt when it may. One restart per autoRestartCooldown.
func (s *Supervisor) claimAutoRestart(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastAutoRestart.IsZero() && now.Sub(s.lastAutoRestart) < autoRestartCooldown {
		return false
	}
	s.lastAutoRestart = now
	return true
}

// Restart kills the current librespot process; the supervisor relaunches it
// (after its backoff), forcing a fresh Spotify Connect registration. Used to
// bring the device back when it has dropped off Spotify's device list while
// idle.
func (s *Supervisor) Restart() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Stop terminates librespot and stops supervising.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	stop, cmd := s.stop, s.cmd
	s.mu.Unlock()

	close(stop)
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
