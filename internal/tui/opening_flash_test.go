package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// initCommands is how many commands Init batches. Shape only: running them
// would sit out the flash's real tick.
func initCommands(t *testing.T, m Model) int {
	t.Helper()
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init returned no batch")
	}
	return len(batch)
}

func TestBuild_OpeningFlash(t *testing.T) {
	plain := initCommands(t, Build(Deps{Lister: fakeLister{}}))

	t.Run("the picker opens on it, and Init schedules its tick", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Flash: "session gone"})

		if m.flashText != "session gone" || m.flashKind != flashWarning {
			t.Fatalf("flash = %q, kind %v; want the warning it opens on", m.flashText, m.flashKind)
		}
		if got := initCommands(t, m); got != plain+1 {
			t.Errorf("Init batches %d commands, want %d: the plain picker's and the flash's tick", got, plain+1)
		}
	})

	t.Run("its tick clears it, and only its own", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Flash: "session gone"})

		stale, _ := m.Update(flashTickMsg{Gen: m.flashGen + 1})
		if got := stale.(Model).flashText; got != "session gone" {
			t.Errorf("flash after another flash's tick = %q, want it kept", got)
		}
		cleared, _ := m.Update(flashTickMsg{Gen: m.flashGen})
		if got := cleared.(Model).flashText; got != "" {
			t.Errorf("flash after its tick = %q, want cleared", got)
		}
	})

	t.Run("a capture's seeded flash schedules no tick", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Capture: CaptureSeeds{Flash: "seeded"}})

		if got := initCommands(t, m); got != plain {
			t.Errorf("Init batches %d commands, want the plain picker's %d: a fixture's frame holds its flash", got, plain)
		}
	})
}
