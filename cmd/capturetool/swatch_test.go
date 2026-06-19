package main

import (
	"testing"
)

// TestResolveProgramContrastValidation verifies the capture tool resolves the
// contrast-validation fixture into a runnable swatch tea.Model. The swatch is the
// §16.5 lock-in/bail validation surface — a labelled tint swatch on the owned
// canvas, deliberately NOT the production tui.Model (the four tint SURFACES land
// in later phases). It is resolved as a tea.Model so vhs can drive it.
func TestResolveProgramContrastValidation(t *testing.T) {
	t.Run("light pins the swatch", func(t *testing.T) {
		m, err := resolveProgram("contrast-validation", "light")
		if err != nil {
			t.Fatalf("resolveProgram(contrast-validation, light): %v", err)
		}
		if m == nil {
			t.Fatal("resolveProgram returned a nil model")
		}
	})

	t.Run("dark pins the swatch", func(t *testing.T) {
		if _, err := resolveProgram("contrast-validation", "dark"); err != nil {
			t.Fatalf("resolveProgram(contrast-validation, dark): %v", err)
		}
	})

	t.Run("invalid appearance is an error", func(t *testing.T) {
		if _, err := resolveProgram("contrast-validation", "purple"); err == nil {
			t.Fatal("resolveProgram(contrast-validation, purple) returned nil error, want error")
		}
	})
}

// TestResolveProgramSessionsFixture verifies the existing sessions-flat fixture
// still resolves through the same dispatch (a tui.Model is a tea.Model), so the
// swatch branch is additive and does not regress the production capture path.
func TestResolveProgramSessionsFixture(t *testing.T) {
	m, err := resolveProgram("sessions-flat", "dark")
	if err != nil {
		t.Fatalf("resolveProgram(sessions-flat, dark): %v", err)
	}
	if m == nil {
		t.Fatal("resolveProgram returned a nil model")
	}
}
