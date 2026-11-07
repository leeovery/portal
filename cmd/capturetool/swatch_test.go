package main

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/themetest"
)

func TestResolveProgramContrastValidation(t *testing.T) {
	t.Run("a built-in slug pins the swatch", func(t *testing.T) {
		m, err := resolveProgram(capture.ContrastValidationFixture, "tokyo-night", io.Discard, false)
		if err != nil {
			t.Fatalf("resolveProgram(contrast-validation, tokyo-night): %v", err)
		}
		if m == nil {
			t.Fatal("resolveProgram returned a nil model")
		}

		want := themetest.Builtin(t, "tokyo-night")
		title := fmt.Sprintf("CONTRAST VALIDATION — canvas %s", want.Canvas.Value)
		if content := m.View().Content; !strings.Contains(content, title) {
			t.Errorf("the swatch does not render tokyo-night's palette: no %q in its view\n--- view ---\n%s", title, content)
		}
	})

	t.Run("an explicit path pins the swatch too", func(t *testing.T) {
		path := themetest.WriteWithCanvas(t, t.TempDir(), "mytheme.theme", "#1a2b3c")

		m, err := resolveProgram(capture.ContrastValidationFixture, path, io.Discard, false)
		if err != nil {
			t.Fatalf("resolveProgram(contrast-validation, %q): %v", path, err)
		}

		const title = "CONTRAST VALIDATION — canvas #1A2B3C"
		if content := m.View().Content; !strings.Contains(content, title) {
			t.Errorf("the swatch does not render the file's palette: no %q in its view\n--- view ---\n%s", title, content)
		}
	})

}

func TestResolveProgramSessionsFixture(t *testing.T) {
	m, err := resolveProgram("sessions-flat", defaultThemeSlug, io.Discard, false)
	if err != nil {
		t.Fatalf("resolveProgram(sessions-flat, %s): %v", defaultThemeSlug, err)
	}
	if m == nil {
		t.Fatal("resolveProgram returned a nil model")
	}
}
