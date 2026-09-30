//go:build darwin || linux

package cmd

import (
	"os"
	"testing"
	"time"
)

func TestTTYDrain(t *testing.T) {
	t.Run("it errors for a non-tty descriptor", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe() error = %v", err)
		}
		t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

		if err := resumeInputDrain.drain(int(r.Fd())); err == nil {
			t.Error("drain on a pipe returned nil, want an error")
		}
	})

	t.Run("it settles for as long as the waiter waits for the pane to fall quiet and gives up after one second", func(t *testing.T) {
		if resumeInputDrain.settle != resumeInputQuiet {
			t.Errorf("settle = %v, want %v", resumeInputDrain.settle, resumeInputQuiet)
		}
		if resumeInputDrain.bound != time.Second {
			t.Errorf("bound = %v, want 1s", resumeInputDrain.bound)
		}
	})
}
