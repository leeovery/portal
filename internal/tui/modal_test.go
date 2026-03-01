package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderModal(t *testing.T) {
	t.Run("overlay contains modal content and list view", func(t *testing.T) {
		content := "Kill dev? (y/n)"
		listView := "Sessions\n> dev  3 windows\n  work  1 window"

		result := renderModal(content, listView, 80, 24)

		if !strings.Contains(result, content) {
			t.Errorf("renderModal output should contain modal content %q, got:\n%s", content, result)
		}
		if !strings.Contains(result, "Sessions") {
			t.Errorf("renderModal output should contain list view text, got:\n%s", result)
		}
		if !strings.Contains(result, "work") {
			t.Errorf("renderModal output should contain list view items, got:\n%s", result)
		}
	})

	t.Run("overlay is non-empty string", func(t *testing.T) {
		content := "Kill dev? (y/n)"
		listView := "Sessions\n> dev  3 windows"

		result := renderModal(content, listView, 80, 24)

		if result == "" {
			t.Error("renderModal should produce non-empty output")
		}
	})

	t.Run("overlay differs from plain list view", func(t *testing.T) {
		content := "Kill dev? (y/n)"
		listView := "Sessions\n> dev  3 windows"

		result := renderModal(content, listView, 80, 24)

		if result == listView {
			t.Error("renderModal output should differ from plain list view")
		}
	})

	t.Run("modal content has border styling", func(t *testing.T) {
		content := "Confirm action"
		listView := "some list"

		result := renderModal(content, listView, 80, 24)

		// The modal content should be wrapped in a border (lipgloss borders use box-drawing characters)
		// Check that border characters are present
		hasBorder := strings.ContainsAny(result, "─│┌┐└┘├┤┬┴┼╭╮╰╯")
		if !hasBorder {
			t.Errorf("renderModal output should contain border characters, got:\n%s", result)
		}
	})

	t.Run("modal centers correctly over ANSI-styled background", func(t *testing.T) {
		// Build a background with raw ANSI escape sequences.
		// \033[1;31m is bold red, \033[0m is reset.
		// These occupy bytes/runes but are zero-width on screen.
		// The visible text "styled session line" is 19 chars wide,
		// but the full string with ANSI escapes is much longer in runes.
		ansiPrefix := "\033[1;31m"
		ansiSuffix := "\033[0m"
		visibleText := "styled session line"
		styledLine := ansiPrefix + visibleText + ansiSuffix

		// Verify our ANSI setup: rune count differs from display width
		if len([]rune(styledLine)) == ansi.StringWidth(styledLine) {
			t.Fatal("test setup: styled line rune count should differ from display width")
		}

		var bgLines []string
		for i := 0; i < 24; i++ {
			bgLines = append(bgLines, styledLine)
		}
		listView := strings.Join(bgLines, "\n")

		content := "OK"
		width := 40
		height := 24

		result := renderModal(content, listView, width, height)

		// The styled modal content is rendered with border + padding.
		styledModal := modalStyle.Render(content)
		modalWidth := lipgloss.Width(styledModal)

		// Expected left offset for centering
		expectedX := (width - modalWidth) / 2

		// Find the first line that contains the modal border top (╭)
		resultLines := strings.Split(result, "\n")
		found := false
		for _, line := range resultLines {
			if !strings.Contains(line, "╭") {
				continue
			}
			found = true

			// The modal border should start at expectedX display columns.
			// Find the position of ╭ in display columns (ANSI-aware).
			idx := strings.Index(line, "╭")
			prefix := line[:idx]
			displayOffset := ansi.StringWidth(prefix)

			if displayOffset != expectedX {
				t.Errorf("modal horizontal offset = %d display columns, want %d\nline: %q", displayOffset, expectedX, line)
			}
			break
		}
		if !found {
			t.Errorf("could not find modal border character in result:\n%s", result)
		}
	})
}
