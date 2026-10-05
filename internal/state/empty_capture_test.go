package state_test

import (
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

const emptyCaptureOwnServer = 4242

// countingConfirmer answers every confirmation with pid and counts the reads.
type countingConfirmer struct {
	pid   int
	reads int
}

func (c *countingConfirmer) ConfirmAnswering() (int, error) {
	c.reads++
	return c.pid, nil
}

func seedTranscript(t *testing.T, dir, paneKey, body string) {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("create scrollback dir: %v", err)
	}
	if err := os.WriteFile(state.ScrollbackFile(dir, paneKey), []byte(body), 0o600); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}
}

func TestConfirmEmptyCaptureSendsNoReadWhenNothingSavedCanBeLost(t *testing.T) {
	cases := []struct {
		name  string
		saved *string
		data  string
	}{
		{"a non-empty capture over a saved transcript", new("saved"), "captured"},
		{"an empty capture with no saved transcript", nil, ""},
		{"an empty capture over an empty saved transcript", new(""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.saved != nil {
				seedTranscript(t, dir, "work__0_0", *tc.saved)
			}
			c := &countingConfirmer{pid: emptyCaptureOwnServer + 1}

			if err := state.ConfirmEmptyCapture(c, emptyCaptureOwnServer, dir, "work__0_0", []byte(tc.data)); err != nil {
				t.Errorf("ConfirmEmptyCapture = %v, want nil", err)
			}
			if c.reads != 0 {
				t.Errorf("confirmation reads = %d, want none", c.reads)
			}
		})
	}
}

func TestConfirmEmptyCaptureRefusesWhenTheSavedTranscriptCannotBeInspected(t *testing.T) {
	dir := t.TempDir()
	seedTranscript(t, dir, "work__0_0", "saved")
	sbDir := state.ScrollbackDir(dir)
	if err := os.Chmod(sbDir, 0o000); err != nil {
		t.Fatalf("deny scrollback dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sbDir, 0o700) })
	c := &countingConfirmer{pid: emptyCaptureOwnServer + 1}

	err := state.ConfirmEmptyCapture(c, emptyCaptureOwnServer, dir, "work__0_0", nil)

	if !errors.Is(err, state.ErrUnconfirmedEmptyCapture) || !errors.Is(err, state.ErrNotOwnServer) {
		t.Errorf("ConfirmEmptyCapture = %v, want ErrUnconfirmedEmptyCapture wrapping ErrNotOwnServer", err)
	}
	if c.reads != 1 {
		t.Errorf("confirmation reads = %d, want 1", c.reads)
	}
}
