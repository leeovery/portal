package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// stagedMintBandModel is a command-pending Projects model whose concurrent
// bootstrap is still running, sized so the band's rows are measurable.
func stagedMintBandModel(t *testing.T, width int) Model {
	t.Helper()
	m := noticeBandModel("alpha-row")
	m.termWidth = width
	m.commandPending = true
	m.command = []string{"npm", "run", "dev"}
	m.activePage = PageProjects
	m.progressReceiver = tea.Cmd(func() tea.Msg { return BootstrapProgressMsg{Index: 1} })
	m.resyncPageLayouts()
	return m
}

func TestStagedMintProjectBand(t *testing.T) {
	t.Run("it reports the wait in the projects band while a mint is staged, and shows the pending-command banner again once it is issued", func(t *testing.T) {
		m := stagedMintBandModel(t, 80)

		if role, message, ok := m.activeProjectNoticeBand(); !ok || role != bandCommand || message != commandBandText {
			t.Fatalf("pre-stage band = (%v, %q, %v), want the pending-command banner", role, message, ok)
		}

		(&m).createSession("/tmp/alpha")

		role, message, ok := m.activeProjectNoticeBand()
		if !ok || role != bandInfo || message != stagedMintBandText {
			t.Errorf("staged band = (%v, %q, %v), want the info-role wait %q", role, message, ok, stagedMintBandText)
		}

		var model tea.Model = m
		model, _ = model.Update(BootstrapCompleteMsg{})

		role, message, ok = model.(Model).activeProjectNoticeBand()
		if !ok || role != bandCommand || message != commandBandText {
			t.Errorf("post-issue band = (%v, %q, %v), want the pending-command banner back", role, message, ok)
		}
	})

	t.Run("it keeps a live flash ahead of the wait band", func(t *testing.T) {
		m := stagedMintBandModel(t, 80)
		(&m).createSession("/tmp/alpha")
		m.setFlash("__ORDINARY__")

		wantRole, wantMessage, wantOK := m.flashSlotClaim()
		if !wantOK {
			t.Fatal("the shared claim did not take the slot; the fixture is wrong")
		}
		role, message, ok := m.activeProjectNoticeBand()
		if role != wantRole || message != wantMessage || ok != wantOK {
			t.Errorf("Projects band = (%v, %q, %v), want the shared claim's (%v, %q, %v)", role, message, ok, wantRole, wantMessage, wantOK)
		}
	})

	t.Run("it reserves the wait band's rows in the projects list budget", func(t *testing.T) {
		// Narrow enough that the wait band wraps past the one-row banner, so a
		// missed re-measure leaves the list visibly over budget.
		m := stagedMintBandModel(t, 34)
		before := m.projectBandHeight()

		(&m).createSession("/tmp/alpha")

		staged := m.projectBandHeight()
		if staged <= before {
			t.Fatalf("staged band height = %d, want more than the banner's %d; the fixture no longer wraps", staged, before)
		}
		if got := lipgloss.Height(m.renderProjectBandSlot()); got != staged {
			t.Errorf("projectBandHeight = %d, want the rendered slot's %d", staged, got)
		}
		assertProjectListBudget(t, m, "staging")

		var model tea.Model = m
		model, _ = model.Update(BootstrapCompleteMsg{})
		assertProjectListBudget(t, model.(Model), "issuing")
	})
}

func assertProjectListBudget(t *testing.T, m Model, when string) {
	t.Helper()
	want := m.contentHeight() - m.headerHeight(m.contentWidth()) - m.projectFooterHeight(m.contentWidth()) - m.projectBandHeight()
	if got := m.projectList.Height(); got != want {
		t.Errorf("projects list height after %s = %d, want %d — the budget was not re-measured", when, got, want)
	}
}
