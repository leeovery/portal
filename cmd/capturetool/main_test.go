package main

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui"
)

// TestResolveModel verifies the capture tool resolves a fixture name into a real
// production tui.Model via the shared tui.Build constructor — without opening a
// tmux server or touching config (the fixture is fully in-memory).
func TestResolveModel(t *testing.T) {
	t.Run("known fixture builds a sessions-page model", func(t *testing.T) {
		m, err := resolveModel("sessions-flat", "dark")
		if err != nil {
			t.Fatalf("resolveModel(sessions-flat): %v", err)
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})

	t.Run("unknown fixture is an error that lists the available fixtures", func(t *testing.T) {
		_, err := resolveModel("nope", "dark")
		if err == nil {
			t.Fatal("resolveModel(nope) returned nil error, want error")
		}
		if !strings.Contains(err.Error(), "sessions-flat") {
			t.Errorf("error %q does not list the available fixtures", err.Error())
		}
	})

	t.Run("empty fixture name is an error", func(t *testing.T) {
		if _, err := resolveModel("", "dark"); err == nil {
			t.Fatal("resolveModel(\"\") returned nil error, want error")
		}
	})

	t.Run("invalid appearance is an error", func(t *testing.T) {
		if _, err := resolveModel("sessions-flat", "purple"); err == nil {
			t.Fatal("resolveModel with appearance=purple returned nil error, want error")
		}
	})
}

// TestResolveAppearance verifies the --appearance flag maps to the pinned
// prefs.Appearance the model resolves the owned canvas from. The harness drives
// the pin path (no OSC 11 detection / no first-paint wait), so the captured
// canvas is deterministic while still exercising the real §2.6 resolution.
func TestResolveAppearance(t *testing.T) {
	t.Run("dark pins AppearanceDark", func(t *testing.T) {
		got, err := resolveAppearance("dark")
		if err != nil {
			t.Fatalf("resolveAppearance(dark): %v", err)
		}
		if got != prefs.AppearanceDark {
			t.Errorf("resolveAppearance(dark) = %v, want AppearanceDark", got)
		}
	})

	t.Run("light pins AppearanceLight", func(t *testing.T) {
		got, err := resolveAppearance("light")
		if err != nil {
			t.Fatalf("resolveAppearance(light): %v", err)
		}
		if got != prefs.AppearanceLight {
			t.Errorf("resolveAppearance(light) = %v, want AppearanceLight", got)
		}
	})

	t.Run("invalid value is an error", func(t *testing.T) {
		if _, err := resolveAppearance("purple"); err == nil {
			t.Fatal("resolveAppearance(purple) returned nil error, want error")
		}
	})
}
