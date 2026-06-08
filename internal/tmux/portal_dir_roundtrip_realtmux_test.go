package tmux_test

// Real-tmux round-trip guard for the @portal-dir stamp seam.
//
// The @portal-dir stamp has two halves that are each unit-tested in isolation:
// CreateFromDir's SetSessionOption(@portal-dir, dir) WRITE, and ListSessions
// parsing #{@portal-dir} from a fabricated commander line (the READ). Neither
// unit test exercises the round trip through a real tmux server, so a tmux
// quoting/escaping or format-string drift could pass both halves while silently
// breaking grouping in production — the stamped value and the format-field read
// would disagree only against a live server.
//
// This test closes that seam: it stamps @portal-dir via the SAME production
// SetSessionOption method CreateFromDir uses, reads it back via the production
// ListSessions, and asserts Session.Dir equals the stamped value exactly —
// including a path containing a space, the most likely tmux quoting/format
// drift trigger. Like the other real-tmux guards in this package it carries NO
// build tag and is gated only by SkipIfNoTmux(t) so tmux-less environments skip
// cleanly rather than fail.
//
// Spec: .workflows/session-tagging-and-grouping/specification §
// "The stamp (fast path)" — the stamped value and the ListSessions read must
// agree.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// portalDirOption is the literal session user-option name the production stamp
// uses (session.PortalDirOption = "@portal-dir"). It is repeated here as a
// literal to avoid an import for a single string and must stay byte-identical
// to the production constant.
const portalDirOption = "@portal-dir"

// findSessionDir returns the Dir of the session named name from a ListSessions
// result, fatalling the test if the session is not present.
func findSessionDir(t *testing.T, sessions []tmux.Session, name string) string {
	t.Helper()
	for _, s := range sessions {
		if s.Name == name {
			return s.Dir
		}
	}
	t.Fatalf("session %q not found in ListSessions result %+v", name, sessions)
	return ""
}

// TestPortalDirStampRoundTrip drives the @portal-dir stamp value through a real
// tmux write→read round trip. For each stamped value it creates a session on an
// isolated socket, stamps @portal-dir via the production SetSessionOption (the
// exact write CreateFromDir performs), reads it back via the production
// ListSessions, and asserts Session.Dir equals the stamped value byte-for-byte.
//
// The space-containing path is the load-bearing case: a tmux quoting or
// format-string drift in the set-option write or the #{@portal-dir} read would
// corrupt a path with a space while leaving simple paths intact, so the space
// case is the one most likely to expose drift that the isolated unit tests
// cannot see.
func TestPortalDirStampRoundTrip(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	cases := []struct {
		name        string
		sessionName string
		stamp       string
	}{
		{
			name:        "plain path",
			sessionName: "rt-plain",
			stamp:       "/code/portal",
		},
		{
			name:        "path with a space",
			sessionName: "rt-space",
			// A space is the most likely tmux quoting/format-string drift
			// trigger; the stamp must survive the set-option write and the
			// #{@portal-dir} read intact.
			stamp: "/code/my project",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := tmuxtest.New(t, "portaldir-")
			client := ts.Client()

			// The session needs a real, existing cwd for new-session -c; the
			// @portal-dir stamp value is an independent option string, so the
			// stamp (including the space case) need not exist on disk.
			cwd := t.TempDir()
			if err := client.NewSession(tc.sessionName, cwd, ""); err != nil {
				t.Fatalf("NewSession(%q): %v", tc.sessionName, err)
			}
			ts.WaitForSession(t, tc.sessionName, 2*time.Second)

			// Stamp via the SAME production write CreateFromDir uses.
			if err := client.SetSessionOption(tc.sessionName, portalDirOption, tc.stamp); err != nil {
				t.Fatalf("SetSessionOption(%q, %q, %q): %v", tc.sessionName, portalDirOption, tc.stamp, err)
			}

			// Read back via the production ListSessions and assert exact
			// round-trip equality: this proves the stamped value survives the
			// tmux write→#{@portal-dir} read intact.
			sessions, err := client.ListSessions()
			if err != nil {
				t.Fatalf("ListSessions: %v", err)
			}
			if got := findSessionDir(t, sessions, tc.sessionName); got != tc.stamp {
				t.Errorf("round-trip Dir = %q, want %q (tmux quoting/format-string drift)", got, tc.stamp)
			}
		})
	}
}

// TestPortalDirStampRoundTrip_TempDirWithSpace round-trips a real t.TempDir()
// path that itself contains a space, exercising the space-drift guard against
// an actual on-disk directory used as both the session cwd and the stamp value
// — the closest analogue to the production CreateFromDir flow where the stamp
// is the resolved working directory.
func TestPortalDirStampRoundTrip_TempDirWithSpace(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "portaldir-")
	client := ts.Client()

	dir := filepath.Join(t.TempDir(), "dir with space")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create dir with space: %v", err)
	}

	const sessionName = "rt-tempspace"
	if err := client.NewSession(sessionName, dir, ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalDirOption, dir); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalDirOption, dir, err)
	}

	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got := findSessionDir(t, sessions, sessionName); got != dir {
		t.Errorf("round-trip Dir = %q, want %q (real space-containing path must survive intact)", got, dir)
	}
}

// TestPortalDirStampRoundTrip_UnstampedSessionEmptyDir asserts that a session
// with no @portal-dir option set parses to an empty Dir over the real socket —
// the production "no stamp" signal (e.g. restored post-reboot). Confirms the
// #{@portal-dir} read yields an empty trailing field rather than spurious
// content when the option is absent.
func TestPortalDirStampRoundTrip_UnstampedSessionEmptyDir(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "portaldir-")
	client := ts.Client()

	const sessionName = "rt-unstamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Deliberately do NOT stamp @portal-dir.
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got := findSessionDir(t, sessions, sessionName); got != "" {
		t.Errorf("unstamped session Dir = %q, want \"\" (absent @portal-dir must parse to empty)", got)
	}
}
