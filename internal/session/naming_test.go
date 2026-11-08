package session_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/session"
)

func TestSanitiseProjectName(t *testing.T) {
	for input, want := range map[string]string{
		"portal":       "portal",
		"my.app:v2":    "my.app:v2",
		"$work":        "$work",
		"-lead":        "lead",
		"_portal":      "portal",
		"-_-x":         "x",
		"a/b":          "a-b",
		"/":            "",
		"line\nbreak":  "line-break",
		"\tlead":       "lead",
		"bad\xffbyte":  "bad-byte",
		"café — 日本":    "café — 日本",
		"a_b-c":        "a_b-c",
		"":             "",
		"\x00\x01\x02": "",
	} {
		if got := session.SanitiseProjectName(input); got != want {
			t.Errorf("SanitiseProjectName(%q) = %q, want %q", input, got, want)
		}
	}
}

func accept(string) error { return nil }

func TestSessionName(t *testing.T) {
	t.Run("it is the project, a hyphen and the suffix", func(t *testing.T) {
		got, err := session.SessionName("portal", "abc123", accept)
		if err != nil || got != "portal-abc123" {
			t.Errorf("SessionName = %q, %v; want portal-abc123", got, err)
		}
	})

	t.Run("a project that sanitises to nothing takes the fallback word, never the bare nanoid", func(t *testing.T) {
		for _, project := range []string{"", "-", "_", "/"} {
			got, err := session.SessionName(project, "abc123", accept)
			if err != nil || got != session.FallbackProject+"-abc123" {
				t.Errorf("SessionName(%q) = %q, %v; want %s-abc123", project, got, err, session.FallbackProject)
			}

		}
	})

	t.Run("it cuts the project part until the name fits", func(t *testing.T) {
		got, err := session.SessionName("abcdefghij", "abc123", maxLen(12))
		if err != nil || got != "abcde-abc123" {
			t.Errorf("SessionName = %q, %v; want abcde-abc123", got, err)
		}
	})

	t.Run("a cut never splits a character", func(t *testing.T) {
		got, err := session.SessionName("日本語", "abc123", maxLen(13))
		if err != nil || got != "日本-abc123" {
			t.Errorf("SessionName = %q, %v; want 日本-abc123", got, err)
		}
	})

	t.Run("a cut never leaves the part ending in a hyphen", func(t *testing.T) {
		got, err := session.SessionName("ab--cd", "abc123", maxLen(10))
		if err != nil || got != "ab-abc123" {
			t.Errorf("SessionName = %q, %v; want ab-abc123", got, err)
		}
	})

	t.Run("with nothing left to cut it returns the refusal", func(t *testing.T) {
		_, err := session.SessionName("portal", "abc123", maxLen(7))
		if !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("SessionName = %v, want ErrInvalidName", err)
		}
	})

	t.Run("it fits a long project within zmx's limit for a long zmx dir", func(t *testing.T) {
		b, err := zmx.New(zmx.Config{Dir: "/Users/someone-with-a-long-name/.local/state/zmx"})
		if err != nil {
			t.Fatalf("zmx.New: %v", err)
		}
		got, err := session.SessionName(strings.Repeat("project", 20), "abc123", b.ValidateName)
		if err != nil {
			t.Fatalf("SessionName: %v", err)
		}
		if len(got) != b.MaxNameLen() || !strings.HasSuffix(got, "-abc123") {
			t.Errorf("SessionName = %q (%d bytes), want %d bytes ending -abc123", got, len(got), b.MaxNameLen())
		}
	})
}

func TestSessionNamesAreOnesZmxAccepts(t *testing.T) {
	hostile := []string{"", "-", "_portal", "-lead", "logs", "a/b", "/", "tab\there", "bad\xff", "star*", "-h", "--labels", "..", "."}
	for _, project := range hostile {
		name, err := session.SessionName(project, "abc123", accept)
		if err != nil {
			t.Fatalf("SessionName(%q): %v", project, err)
		}
		if err := zmx.CheckName(name); err != nil {
			t.Errorf("SessionName(%q) = %q, which zmx refuses: %v", project, name, err)
		}
		if (backend.Session{Name: name}).PortalOwned() {
			t.Errorf("SessionName(%q) = %q, which reads as one of Portal's own", project, name)
		}
	}
}
