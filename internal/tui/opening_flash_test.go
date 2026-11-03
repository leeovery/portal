package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// initMessages runs each command Init batches, each on its own goroutine, and
// returns the messages that arrive within wait.
func initMessages(t *testing.T, m Model, wait time.Duration) []tea.Msg {
	t.Helper()
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init returned no batch")
	}
	msgs := make(chan tea.Msg, len(batch))
	for _, cmd := range batch {
		go func() { msgs <- cmd() }()
	}
	var got []tea.Msg
	deadline := time.After(wait)
	for range batch {
		select {
		case msg := <-msgs:
			got = append(got, msg)
		case <-deadline:
			return got
		}
	}
	return got
}

func flashTicks(msgs []tea.Msg) []flashTickMsg {
	var ticks []flashTickMsg
	for _, msg := range msgs {
		if tick, ok := msg.(flashTickMsg); ok {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

func TestBuild_OpeningFlash(t *testing.T) {
	t.Run("the picker opens on it, and its tick clears it", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Flash: "session gone"})
		if m.flashText != "session gone" || m.flashKind != flashWarning {
			t.Fatalf("flash = %q, kind %v; want the warning it opens on", m.flashText, m.flashKind)
		}

		ticks := flashTicks(initMessages(t, m, flashAutoClearDuration+5*time.Second))
		if len(ticks) != 1 {
			t.Fatalf("Init scheduled %d flash ticks, want 1", len(ticks))
		}
		updated, _ := m.Update(ticks[0])
		if got := updated.(Model).flashText; got != "" {
			t.Errorf("flash after its tick = %q, want cleared", got)
		}
	})

	t.Run("without one, Init schedules no tick", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}})

		if ticks := flashTicks(initMessages(t, m, flashAutoClearDuration+time.Second)); len(ticks) != 0 {
			t.Errorf("Init scheduled %d flash ticks, want none", len(ticks))
		}
	})

	t.Run("a capture's seeded flash schedules no tick either", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Capture: CaptureSeeds{Flash: "seeded"}})

		if ticks := flashTicks(initMessages(t, m, flashAutoClearDuration+time.Second)); len(ticks) != 0 {
			t.Errorf("Init scheduled %d flash ticks for a seeded flash, want none: a fixture's frame holds it", len(ticks))
		}
	})
}
