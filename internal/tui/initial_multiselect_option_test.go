package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

func multiSelectFixtureSessions() []Session {
	return []Session{
		{Name: "agentic-workflows-code-based", Attached: true},
		{Name: "agentic-workflows-codify"},
		{Name: "fab-flowx-explore"},
		{Name: "evvi webhooks and watchers"},
		{Name: "aviva-proxy-qNyfEO"},
		{Name: "designlab-web-r8suyU"},
		{Name: "evvi-sync-engine"},
		{Name: "fab-aws-migration"},
		{Name: "flow-v1-api-XkkhTN"},
		{Name: "flowx-7UKPZH"},
		{Name: "fabric-lk26UG"},
		{Name: "folio-Jiz4el"},
	}
}

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
