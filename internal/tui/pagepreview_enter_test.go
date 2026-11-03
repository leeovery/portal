package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backendtest"
)

func TestPreviewEnter_AsksToOpenThePreviewedSession(t *testing.T) {
	m := newTestPreview(t, "content", 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if cmd == nil {
		t.Fatalf("Enter returned no command, want one asking to open the session")
	}
	if got, ok := cmd().(previewOpenMsg); !ok || got.Session != "work" {
		t.Errorf("Enter's message = %#v, want previewOpenMsg for work", cmd())
	}
}

func TestPreviewEnter_NotForwardedToViewport(t *testing.T) {
	m := newTestPreview(t, strings.Repeat("line\r\n", 50), 80, 10)
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if updated.viewport.YOffset() != before {
		t.Errorf("viewport.YOffset = %d, want unchanged %d: Enter must not reach the viewport", updated.viewport.YOffset(), before)
	}
}

func TestPreviewEnter_OpensWhateverTheBodyShows(t *testing.T) {
	for name, stage := range map[string]func(*backendtest.Fake){
		"not answering": func(f *backendtest.Fake) { f.StopAnswering("work") },
		"unavailable":   func(f *backendtest.Fake) { f.SetHistory("work", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			fake := historyFake(t, "x", "work")
			stage(fake)
			m, ok := NewPreviewModel("work", newPreviewHistory(fake), 80, 24)
			if !ok {
				t.Fatalf("setup: NewPreviewModel ok=false")
			}

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

			if cmd == nil {
				t.Fatalf("Enter returned no command")
			}
			if _, ok := cmd().(previewOpenMsg); !ok {
				t.Errorf("Enter's message = %#v, want previewOpenMsg", cmd())
			}
		})
	}
}

// The picker's normal path: the name is recorded and the TUI quits, so the
// connector runs only after the program has stopped.
func TestPreviewOpenMsg_SelectsTheSessionAndQuits(t *testing.T) {
	m := modelWithSeams(t, []Session{{Name: "alpha"}, {Name: "bravo"}}, historyFake(t, "x", "alpha", "bravo"))
	m = asModel(t, dropCmd(m.Update(keySpaceMsg())))

	updated, cmd := m.Update(previewOpenMsg{Session: "alpha"})

	if got := asModel(t, updated).Selected(); got != "alpha" {
		t.Errorf("Selected() = %q, want alpha", got)
	}
	if cmd == nil {
		t.Fatalf("previewOpenMsg returned no command, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("previewOpenMsg's command = %#v, want tea.Quit", cmd())
	}
}
