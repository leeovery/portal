package capture_test

import (
	"testing"

	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/tui"
)

// TestFixtureByName resolves the named fixtures and verifies the deterministic
// session set wired into the sessions-flat fixture matches the Paper-mock list
// (spec § 15 / tick task), in order, with the correct window counts and
// attached flags.
func TestFixtureByName(t *testing.T) {
	t.Run("unknown fixture is reported as an error", func(t *testing.T) {
		if _, err := capture.FixtureByName("does-not-exist"); err == nil {
			t.Fatal("FixtureByName(unknown) returned nil error, want error")
		}
	})

	t.Run("sessions-flat carries the deterministic Paper-mock session set", func(t *testing.T) {
		fx, err := capture.FixtureByName("sessions-flat")
		if err != nil {
			t.Fatalf("FixtureByName(sessions-flat): %v", err)
		}

		sessions, err := fx.Lister.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}

		type want struct {
			name     string
			windows  int
			attached bool
		}
		// The exact ordered set named in the tick task (load-bearing for the
		// deterministic capture).
		wants := []want{
			{"agentic-workflows-code-based", 3, true},
			{"agentic-workflows-codify", 2, false},
			{"fab-flowx-explore", 1, false},
			{"evvi webhooks and watchers", 4, false},
			{"aviva-proxy-qNyfEO", 1, false},
			{"designlab-web-r8suyU", 2, false},
			{"evvi-sync-engine", 1, false},
			{"fab-aws-migration", 5, false},
			{"flow-v1-api-XkkhTN", 1, false},
			{"flowx-7UKPZH", 2, false},
			{"fabric-lk26UG", 1, false},
			{"folio-Jiz4el", 1, false},
		}

		if len(sessions) != len(wants) {
			t.Fatalf("ListSessions returned %d sessions, want %d", len(sessions), len(wants))
		}
		for i, w := range wants {
			got := sessions[i]
			if got.Name != w.name {
				t.Errorf("session[%d].Name = %q, want %q", i, got.Name, w.name)
			}
			if got.Windows != w.windows {
				t.Errorf("session[%d].Windows = %d, want %d (%s)", i, got.Windows, w.windows, w.name)
			}
			if got.Attached != w.attached {
				t.Errorf("session[%d].Attached = %t, want %t (%s)", i, got.Attached, w.attached, w.name)
			}
		}
	})

	t.Run("fixture deps build a real model via the shared tui.Build constructor", func(t *testing.T) {
		fx, err := capture.FixtureByName("sessions-flat")
		if err != nil {
			t.Fatalf("FixtureByName(sessions-flat): %v", err)
		}

		// The fixture exposes its seam set as a tui.Deps so the harness builds
		// the production model — no bespoke render path.
		m := tui.Build(fx.Deps())
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})
}

// TestFakeSeamsAreInert verifies the mutating fakes are no-ops (the harness must
// never mutate any tmux/server/config state) and the read seams return canned
// data without touching a real tmux server.
func TestFakeSeamsAreInert(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-flat")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-flat): %v", err)
	}
	d := fx.Deps()

	if err := d.Killer.KillSession("anything"); err != nil {
		t.Errorf("Killer.KillSession returned %v, want nil (no-op)", err)
	}
	if err := d.Renamer.RenameSession("a", "b"); err != nil {
		t.Errorf("Renamer.RenameSession returned %v, want nil (no-op)", err)
	}
	if _, err := d.Creator.CreateFromDir("/x", nil); err != nil {
		t.Errorf("Creator.CreateFromDir returned %v, want nil (no-op)", err)
	}

	// The enumerator and reader return canned data deterministically.
	groups, err := d.Enumerator.ListWindowsAndPanesInSession("agentic-workflows-code-based")
	if err != nil {
		t.Errorf("Enumerator returned %v, want nil", err)
	}
	if len(groups) == 0 {
		t.Error("Enumerator returned no window groups, want canned data")
	}
	if _, err := d.Reader.Tail("any-pane-key"); err != nil {
		t.Errorf("Reader.Tail returned %v, want nil", err)
	}
}
