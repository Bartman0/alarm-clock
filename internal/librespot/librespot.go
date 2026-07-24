// Package librespot supervises a librespot process so the Raspberry Pi appears
// as a Spotify Connect device. The user activates the device once from their
// Spotify app (Zeroconf); credentials are cached so it re-registers on boot.
// Playback is then driven via the Spotify Web API (see package spotify).
package librespot

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Supervisor starts librespot and restarts it if it exits.
type Supervisor struct {
	name     string
	cacheDir string

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	stop    chan struct{}
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
		)
		s.mu.Lock()
		s.cmd = cmd
		s.mu.Unlock()

		if err := cmd.Start(); err != nil {
			log.Printf("librespot: start failed: %v", err)
		} else if err := cmd.Wait(); err != nil {
			log.Printf("librespot: exited: %v", err)
		}

		// Back off before restarting, unless we're stopping.
		select {
		case <-s.stop:
			return
		case <-time.After(2 * time.Second):
		}
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
