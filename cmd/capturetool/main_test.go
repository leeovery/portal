package main

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/tui/theme"
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

// TestResolveMode verifies the --appearance flag maps to the owned-canvas
// theme.Mode the model paints (detection 1-7 is not landed, so the harness
// injects the mode explicitly).
func TestResolveMode(t *testing.T) {
	t.Run("dark resolves to theme.Dark", func(t *testing.T) {
		mode, err := resolveMode("dark")
		if err != nil {
			t.Fatalf("resolveMode(dark): %v", err)
		}
		if mode != theme.Dark {
			t.Errorf("resolveMode(dark) = %v, want theme.Dark", mode)
		}
	})

	t.Run("light resolves to theme.Light", func(t *testing.T) {
		mode, err := resolveMode("light")
		if err != nil {
			t.Fatalf("resolveMode(light): %v", err)
		}
		if mode != theme.Light {
			t.Errorf("resolveMode(light) = %v, want theme.Light", mode)
		}
	})

	t.Run("invalid value is an error", func(t *testing.T) {
		if _, err := resolveMode("purple"); err == nil {
			t.Fatal("resolveMode(purple) returned nil error, want error")
		}
	})
}
