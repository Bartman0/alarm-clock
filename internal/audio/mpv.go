// Package audio plays alarm sounds (and later internet radio) by driving an
// mpv subprocess over its JSON IPC socket. When mpv is not installed the player
// degrades to a disabled no-op so the app still runs on a dev machine.
package audio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Player controls a long-lived mpv process via its IPC socket.
type Player struct {
	cmd     *exec.Cmd
	conn    net.Conn
	enabled bool

	mu         sync.Mutex
	fadeCancel chan struct{}
}

// NewPlayer starts mpv in idle mode and connects to its IPC socket. If mpv is
// missing or the socket never appears, it returns a disabled Player (methods
// become no-ops) together with an error the caller can log.
func NewPlayer() (*Player, error) {
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		return &Player{}, fmt.Errorf("mpv not found on PATH: %w", err)
	}

	sock := filepath.Join(os.TempDir(), fmt.Sprintf("alarmclock-mpv-%d.sock", os.Getpid()))
	_ = os.Remove(sock)

	cmd := exec.Command(mpvPath,
		"--idle=yes",
		"--no-video",
		"--no-terminal",
		"--really-quiet",
		"--input-ipc-server="+sock,
	)
	if err := cmd.Start(); err != nil {
		return &Player{}, fmt.Errorf("starting mpv: %w", err)
	}

	var conn net.Conn
	for i := 0; i < 50; i++ {
		if c, e := net.Dial("unix", sock); e == nil {
			conn = c
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if conn == nil {
		_ = cmd.Process.Kill()
		return &Player{}, fmt.Errorf("mpv IPC socket %s never became ready", sock)
	}

	p := &Player{cmd: cmd, conn: conn, enabled: true}
	go p.drain()
	return p, nil
}

// Enabled reports whether the player is backed by a live mpv process.
func (p *Player) Enabled() bool { return p != nil && p.enabled }

// drain consumes mpv's event/response stream so its socket buffer never fills.
func (p *Player) drain() {
	s := bufio.NewScanner(p.conn)
	for s.Scan() {
		// Responses are ignored; commands are fire-and-forget.
	}
}

// send issues a single mpv IPC command.
func (p *Player) send(args ...any) {
	if !p.Enabled() {
		return
	}
	b, err := json.Marshal(map[string]any{"command": args})
	if err != nil {
		return
	}
	b = append(b, '\n')
	p.mu.Lock()
	_, _ = p.conn.Write(b)
	p.mu.Unlock()
}

// PlayLoopFadeIn starts looping the file at zero volume and ramps up to target
// (0–100) over fade. Any in-progress fade is cancelled first.
func (p *Player) PlayLoopFadeIn(path string, fade time.Duration, target int) {
	if !p.Enabled() {
		return
	}
	p.cancelFade()
	p.send("set_property", "volume", 0)
	p.send("set_property", "loop-file", "inf")
	p.send("loadfile", path, "replace")

	cancel := make(chan struct{})
	p.mu.Lock()
	p.fadeCancel = cancel
	p.mu.Unlock()
	go p.ramp(cancel, fade, target)
}

// Stop halts playback and cancels any fade.
func (p *Player) Stop() {
	if !p.Enabled() {
		return
	}
	p.cancelFade()
	p.send("stop")
}

// Close cancels fades, closes the socket and terminates mpv.
func (p *Player) Close() {
	if !p.Enabled() {
		return
	}
	p.cancelFade()
	p.send("quit")
	_ = p.conn.Close()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.enabled = false
}

func (p *Player) ramp(cancel chan struct{}, dur time.Duration, target int) {
	const step = 200 * time.Millisecond
	n := int(dur / step)
	if n < 1 {
		n = 1
	}
	for i := 1; i <= n; i++ {
		select {
		case <-cancel:
			return
		case <-time.After(step):
		}
		p.send("set_property", "volume", target*i/n)
	}
}

func (p *Player) cancelFade() {
	p.mu.Lock()
	if p.fadeCancel != nil {
		close(p.fadeCancel)
		p.fadeCancel = nil
	}
	p.mu.Unlock()
}
