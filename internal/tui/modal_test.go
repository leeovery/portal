package tui

import (
	"strings"
	"testing"
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
}
