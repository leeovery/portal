package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// keypadEnter is the keypad's Enter as Bubble Tea reports it, under the kitty
// keyboard's disambiguation it always asks for and in application keypad mode.
var keypadEnter = tea.KeyPressMsg{Code: tea.KeyKpEnter}

func TestKeypadEnterAsReturn(t *testing.T) {
	cases := []struct {
		name string
		in   tea.Msg
		want tea.Msg
	}{
		{"the keypad's Enter is Return", keypadEnter, tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"a lock's state is dropped", tea.KeyPressMsg{Code: tea.KeyKpEnter, Mod: tea.ModNumLock | tea.ModCapsLock | tea.ModScrollLock}, tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"a modifier held is kept", tea.KeyPressMsg{Code: tea.KeyKpEnter, Mod: tea.ModShift | tea.ModNumLock}, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{"Return is left as it is", tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"another keypad key is left as it is", tea.KeyPressMsg{Code: tea.KeyKp1, Text: "1", Mod: tea.ModNumLock}, tea.KeyPressMsg{Code: tea.KeyKp1, Text: "1", Mod: tea.ModNumLock}},
		{"the keypad Enter's release is left as it is", tea.KeyReleaseMsg{Code: tea.KeyKpEnter}, tea.KeyReleaseMsg{Code: tea.KeyKpEnter}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keypadEnterAsReturn(tc.in); got != tc.want {
				t.Errorf("keypadEnterAsReturn(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// The keypad's Enter is read as Return where every key enters the model, so
// each surface below answers it as it answers Return.
func TestKeypadEnter_ActsAsReturn(t *testing.T) {
	t.Run("on the taken screen it takes the session back", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		m, cmd := pressKeys(m, keypadEnter)

		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want the taken session picked", m.Selected(), quits(cmd))
		}
	})

	t.Run("on the picker it picks the row under the cursor", func(t *testing.T) {
		lister := &rowActionLister{sessions: takenSessions}
		m := rowActionModel(t, lister, nil, nil)

		m, _ = pressKeys(m, tea.KeyPressMsg{Code: tea.KeyDown})
		under := selectedSessionName(t, m)

		m, cmd := pressKeys(m, keypadEnter)

		if m.Selected() != under || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want %q, the row under the cursor, picked", m.Selected(), quits(cmd), under)
		}
	})

	t.Run("in the rename prompt it labels the session", func(t *testing.T) {
		rec := &recordingLabeller{}
		m := newRenameTestModel(rec, "aviva-proxy-qNyfEO", "aviva proxy")

		m, cmd := pressKeys(m, keypadEnter)

		if m.modal != modalNone || cmd == nil {
			t.Fatalf("modal %v, cmd %v; want the prompt closed with a label command", m.modal, cmd != nil)
		}
		cmd()
		if rec.name != "aviva-proxy-qNyfEO" || rec.label != "aviva proxy" {
			t.Errorf("labelled %q as %q, want %q as %q", rec.name, rec.label, "aviva-proxy-qNyfEO", "aviva proxy")
		}
	})

	t.Run("in the theme panel it commits the row under the cursor", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)

		_, _ = pressKeys(m, keypadEnter)

		requireCommitted(t, persister, rows[0].Slug)
	})

	t.Run("on the kill key's prompt it does nothing, as Return does", func(t *testing.T) {
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		m, cmd := pressKeys(m, keypadEnter)

		if m.modal != modalKillConfirm || m.Selected() != "" || cmd != nil || killers.asked.killedName != "" {
			t.Errorf("modal %v, selected %q, cmd %v, killed %q; want the key ignored", m.modal, m.Selected(), cmd != nil, killers.asked.killedName)
		}
	})
}
