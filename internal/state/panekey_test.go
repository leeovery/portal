package state_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

func TestSanitizePaneKey(t *testing.T) {
	t.Run("leaves filesystem-safe session names unchanged", func(t *testing.T) {
		got := state.SanitizePaneKey("work", 0, 1)
		want := "work__0.1"
		if got != want {
			t.Errorf("SanitizePaneKey(%q, %d, %d) = %q; want %q", "work", 0, 1, got, want)
		}
	})

	t.Run("replaces forward slashes in session names", func(t *testing.T) {
		got := state.SanitizePaneKey("foo/bar", 0, 0)
		// Sanitization changes "foo/bar" → "foo_bar"; hash suffix appended.
		if !strings.HasPrefix(got, "foo_bar-") {
			t.Errorf("expected sanitized stem 'foo_bar-' prefix; got %q", got)
		}
		if !strings.HasSuffix(got, "__0.0") {
			t.Errorf("expected suffix '__0.0'; got %q", got)
		}
		if strings.Contains(got, "/") {
			t.Errorf("result must not contain '/'; got %q", got)
		}
	})

	t.Run("replaces a leading dot in session names", func(t *testing.T) {
		got := state.SanitizePaneKey(".hidden", 1, 2)
		if strings.HasPrefix(got, ".") {
			t.Errorf("result must not start with '.'; got %q", got)
		}
		if !strings.HasPrefix(got, "_hidden-") {
			t.Errorf("expected sanitized stem '_hidden-' prefix; got %q", got)
		}
		if !strings.HasSuffix(got, "__1.2") {
			t.Errorf("expected suffix '__1.2'; got %q", got)
		}
	})

	t.Run("replaces null bytes in session names", func(t *testing.T) {
		got := state.SanitizePaneKey("foo\x00bar", 0, 0)
		if strings.ContainsRune(got, '\x00') {
			t.Errorf("result must not contain null byte; got %q", got)
		}
		if !strings.HasPrefix(got, "foo_bar-") {
			t.Errorf("expected sanitized stem 'foo_bar-' prefix; got %q", got)
		}
	})

	t.Run("appends an 8-char hash suffix only when sanitization changed the name", func(t *testing.T) {
		safe := state.SanitizePaneKey("project", 0, 0)
		if safe != "project__0.0" {
			t.Errorf("safe name should have no hash suffix; got %q", safe)
		}

		unsafe := state.SanitizePaneKey("foo/bar", 0, 0)
		// Stem: "foo_bar-XXXXXXXX" (8 hex chars), then "__0.0".
		stem := strings.TrimSuffix(unsafe, "__0.0")
		// stem should be "foo_bar-" + 8 hex chars.
		if !strings.HasPrefix(stem, "foo_bar-") {
			t.Fatalf("expected 'foo_bar-' prefix in stem; got %q", stem)
		}
		hashPart := strings.TrimPrefix(stem, "foo_bar-")
		if len(hashPart) != 8 {
			t.Errorf("expected 8-char hash; got %d chars (%q)", len(hashPart), hashPart)
		}
		for _, r := range hashPart {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				t.Errorf("hash must be lowercase hex; got %q", hashPart)
				break
			}
		}
	})

	t.Run("distinguishes two sessions that sanitize to the same stem via the hash", func(t *testing.T) {
		// Both sanitize to "foo_bar" but originate from different raw inputs.
		a := state.SanitizePaneKey("foo/bar", 0, 0)
		b := state.SanitizePaneKey("foo\x00bar", 0, 0)
		if a == b {
			t.Errorf("expected distinct paneKeys for distinct inputs; both = %q", a)
		}
		// Both sanitized stems should start with "foo_bar-".
		if !strings.HasPrefix(a, "foo_bar-") || !strings.HasPrefix(b, "foo_bar-") {
			t.Errorf("expected both to share sanitized stem; got %q and %q", a, b)
		}
	})

	t.Run("appends window and pane indices as decimal integers", func(t *testing.T) {
		got := state.SanitizePaneKey("work", 12, 34)
		want := "work__12.34"
		if got != want {
			t.Errorf("SanitizePaneKey(%q, %d, %d) = %q; want %q", "work", 12, 34, got, want)
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		a := state.SanitizePaneKey("foo/bar", 3, 4)
		b := state.SanitizePaneKey("foo/bar", 3, 4)
		if a != b {
			t.Errorf("expected determinism; got %q and %q", a, b)
		}
	})
}
