package cmd

import (
	"os"
	"testing"
)

func TestFlushTTYInput(t *testing.T) {
	t.Run("it errors for a non-tty descriptor", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe() error = %v", err)
		}
		t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

		if err := flushTTYInput(int(r.Fd())); err == nil {
			t.Error("flushTTYInput on a pipe returned nil, want an error")
		}
	})
}
