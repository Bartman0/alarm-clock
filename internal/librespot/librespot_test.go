package librespot

import (
	"testing"
	"time"
)

// The lines below are copied verbatim from the Pi's log on 2026-10-01, where
// librespot reconnected cleanly and was then unable to play anything.
func TestUnrecoverableRecognisesTheAudioKeyFailure(t *testing.T) {
	line := "[2026-10-01T08:21:09Z ERROR librespot_core::audio_key] Audio key response timeout"
	if !unrecoverable(line) {
		t.Fatalf("audio key timeout should trigger a restart: %q", line)
	}
}

func TestUnrecoverableIgnoresRecoverableNoise(t *testing.T) {
	benign := []string{
		"[2026-10-01T08:21:06Z WARN  librespot_core::dealer] Websocket connection failed: WebSocket protocol error: Remote sent after having closed",
		"[2026-10-01T08:21:07Z WARN  librespot_connect::spirc] unexpected shutdown",
		"[2026-10-01T08:21:07Z ERROR librespot_core::session] Broken pipe (os error 32)",
		"[2026-10-01T08:21:08Z INFO  librespot_core::session] Authenticated as 'user' !",
		"[2026-10-01T06:15:02Z WARN  librespot_discovery::server] Discovery server failed to start: Address family not supported by protocol (os error 97)",
		"[2026-10-01T08:21:13Z ERROR librespot_playback::player] Unable to read audio file: Symphonia Decoder Error: end of stream",
	}
	for _, line := range benign {
		if unrecoverable(line) {
			t.Errorf("librespot recovers from this on its own; should not restart: %q", line)
		}
	}
}

// The player logs its own follow-up to the same incident, lowercased and
// nested in braces. Only the one ERROR line should count, so a single failure
// does not read as two.
func TestUnrecoverableDoesNotDoubleCountOneIncident(t *testing.T) {
	followUp := "[2026-10-01T08:21:09Z WARN  librespot_playback::player] Unable to load key, continuing without decryption: Operation aborted { audio key response timeout }"
	if unrecoverable(followUp) {
		t.Fatalf("the player's follow-up line should not trigger a second restart: %q", followUp)
	}
}

func TestAutoRestartIsRateLimited(t *testing.T) {
	s := New("Wekker")
	start := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

	if !s.claimAutoRestart(start) {
		t.Fatal("the first restart should be allowed")
	}
	if s.claimAutoRestart(start.Add(autoRestartCooldown - time.Second)) {
		t.Fatal("a second restart inside the cooldown should be refused")
	}
	if !s.claimAutoRestart(start.Add(autoRestartCooldown)) {
		t.Fatal("a restart after the cooldown should be allowed again")
	}
}
