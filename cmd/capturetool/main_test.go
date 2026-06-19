package main

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tui"
)

// TestResolveModel verifies the capture tool resolves a fixture name into a real
// production tui.Model via the shared tui.Build constructor — without opening a
// tmux server or touching config (the fixture is fully in-memory).
func TestResolveModel(t *testing.T) {
	t.Run("known fixture builds a sessions-page model", func(t *testing.T) {
		m, err := resolveModel("sessions-flat")
		if err != nil {
			t.Fatalf("resolveModel(sessions-flat): %v", err)
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})

	t.Run("unknown fixture is an error that lists the available fixtures", func(t *testing.T) {
		_, err := resolveModel("nope")
		if err == nil {
			t.Fatal("resolveModel(nope) returned nil error, want error")
		}
		if !strings.Contains(err.Error(), "sessions-flat") {
			t.Errorf("error %q does not list the available fixtures", err.Error())
		}
	})

	t.Run("empty fixture name is an error", func(t *testing.T) {
		if _, err := resolveModel(""); err == nil {
			t.Fatal("resolveModel(\"\") returned nil error, want error")
		}
	})
}
