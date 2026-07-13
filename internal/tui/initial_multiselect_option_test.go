package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
)

// multiSelectFixtureSessions is the sessions-flat set (same 12 names / order) the
// capture fixture reuses — declared here so the cursor-anchor tests drive the same
// deterministic list the harness renders.
func multiSelectFixtureSessions() []tmux.Session {
	return []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true},
		{Name: "agentic-workflows-codify", Windows: 2},
		{Name: "fab-flowx-explore", Windows: 1},
		{Name: "evvi webhooks and watchers", Windows: 4},
		{Name: "aviva-proxy-qNyfEO", Windows: 1},
		{Name: "designlab-web-r8suyU", Windows: 2},
		{Name: "evvi-sync-engine", Windows: 1},
		{Name: "fab-aws-migration", Windows: 5},
		{Name: "flow-v1-api-XkkhTN", Windows: 1},
		{Name: "flowx-7UKPZH", Windows: 2},
		{Name: "fabric-lk26UG", Windows: 1},
		{Name: "folio-Jiz4el", Windows: 1},
	}
}

// TestWithInitialMultiSelect verifies the capture-only §5 seed seam: the option
// enters multi-select mode with the named sessions pre-marked, and an empty/nil
// name slice is a no-op that leaves the model in normal mode.
func TestWithInitialMultiSelect(t *testing.T) {
	t.Run("seeds multi-select mode and the marked set at construction", func(t *testing.T) {
		names := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
		m := New(fakeLister{}, WithInitialMultiSelect(names))

		if !m.multiSelectMode {
			t.Fatal("multiSelectMode = false, want true (WithInitialMultiSelect must enter the mode)")
		}
		if got, want := len(m.selectedSessions), len(names); got != want {
			t.Fatalf("SelectedSessionCount = %d, want %d", got, want)
		}
		for _, n := range names {
			if _, ok := m.selectedSessions[n]; !ok {
				t.Errorf("selectedSessions missing %q", n)
			}
		}
		// The delegate must reflect the seeded state so the ● arms from the first
		// frame (the list is constructed with a default MultiSelect==false delegate).
		d := m.sessionDelegate()
		if !d.MultiSelect {
			t.Error("sessionDelegate().MultiSelect = false, want true")
		}
		if !isSelected(d.Selected, "fab-flowx-explore") {
			t.Error("sessionDelegate().Selected missing fab-flowx-explore")
		}
	})

	t.Run("nil names leave the model in normal mode", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMultiSelect(nil))
		if m.multiSelectMode {
			t.Error("multiSelectMode = true for nil names, want false (no-op)")
		}
		if m.selectedSessions != nil {
			t.Errorf("selectedSessions = %v for nil names, want nil", m.selectedSessions)
		}
	})

	t.Run("empty names leave the model in normal mode", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMultiSelect([]string{}))
		if m.multiSelectMode {
			t.Error("multiSelectMode = true for empty names, want false (no-op)")
		}
	})
}

// TestWithInitialCursor verifies the capture-only cursor anchor: after the session
// list loads, the highlighted row is the named session (not the default index 0),
// and an empty name is a no-op that leaves the cursor at index 0.
func TestWithInitialCursor(t *testing.T) {
	t.Run("positions the cursor on the named row after items load", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeFlat), WithInitialCursor("fab-flowx-explore"))
		m.applySessionListSize(80, 24)

		u, _ := m.Update(SessionsMsg{Sessions: multiSelectFixtureSessions()})
		mm := u.(Model)
		u2, _ := mm.Update(ProjectsLoadedMsg{Projects: nil})
		mm = u2.(Model)

		si, ok := mm.selectedSessionItem()
		if !ok {
			t.Fatal("no session selected after load")
		}
		if si.Session.Name != "fab-flowx-explore" {
			t.Errorf("selected session = %q, want fab-flowx-explore (cursor anchor)", si.Session.Name)
		}
	})

	t.Run("empty cursor name is a no-op (default index 0)", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeFlat))
		m.applySessionListSize(80, 24)

		u, _ := m.Update(SessionsMsg{Sessions: multiSelectFixtureSessions()})
		mm := u.(Model)
		u2, _ := mm.Update(ProjectsLoadedMsg{Projects: nil})
		mm = u2.(Model)

		si, ok := mm.selectedSessionItem()
		if !ok || si.Session.Name != "agentic-workflows-code-based" {
			t.Errorf("default selection = (%v, %q), want index-0 agentic-workflows-code-based", ok, si.Session.Name)
		}
	})
}
