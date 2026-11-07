package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backendtest"
)

// refreshEvery is the refresh the tests run, short enough to run its ticks.
const refreshEvery = time.Millisecond

// refreshingPreview is a picker on sessions, history answering each with vt,
// previewing alpha, its first read landed, with the tick that read asked for.
func refreshingPreview(t *testing.T, vt string) (Model, *backendtest.Fake, tea.Cmd) {
	t.Helper()
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	history := historyFor(t, vt, sessions)
	m := modelWithSeams(t, sessions, history)
	m.previewRefresh = refreshEvery
	updated, read := m.Update(keySpaceMsg())
	updated, tick := asModel(t, updated).Update(read())
	return asModel(t, updated), history, tick
}

// tickOf runs cmd, which must be a preview's tick, and is its message.
func tickOf(t *testing.T, cmd tea.Cmd) previewTickMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no tick asked for")
	}
	msg, ok := cmd().(previewTickMsg)
	if !ok {
		t.Fatalf("cmd answered %T, want a preview tick", msg)
	}
	return msg
}

func TestPreviewRefresh(t *testing.T) {
	t.Run("a body landing asks for the next read of its open", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "hi")

		if got := tickOf(t, tick); got.gen != m.previewGen {
			t.Errorf("tick for gen %d, want the open's %d", got.gen, m.previewGen)
		}
		if calls := historyCalls(history); len(calls) != 1 {
			t.Errorf("History read %v before the tick, want once", calls)
		}
	})

	t.Run("each tick reads the session again and shows what it reads", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "before")
		history.SetHistory("alpha", []byte("after"))

		updated, read := m.Update(tickOf(t, tick))
		m = asModel(t, updated)
		body, ok := read().(previewBodyMsg)
		if !ok {
			t.Fatal("the tick's command is no read")
		}
		updated, next := m.Update(body)

		if got := lastBodyRow(asModel(t, updated).preview); got != "after" {
			t.Errorf("body = %q, want the session as it reads now", got)
		}
		if calls := historyCalls(history); len(calls) != 2 || calls[1] != "alpha" {
			t.Errorf("History read %v, want alpha read again", calls)
		}
		tickOf(t, next)
	})

	t.Run("a read in flight is the only one: its tick asks for none until its body lands", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "hi")

		_, read := m.Update(tickOf(t, tick))
		if msg, ok := read().(previewBodyMsg); !ok {
			t.Fatalf("the tick's command answered %T, want only its read", msg)
		}
		if calls := historyCalls(history); len(calls) != 2 {
			t.Errorf("History read %v, want the open's read and the tick's", calls)
		}
	})

	t.Run("no tick follows a body when the preview reads once", func(t *testing.T) {
		sessions := []Session{{Name: "alpha"}}
		m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
		updated, read := m.Update(keySpaceMsg())

		if _, next := asModel(t, updated).Update(read()); next != nil {
			t.Error("a body asked for a refresh with none set")
		}
	})

	t.Run("closing the preview stops it", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "hi")
		m = dismissPreview(t, m)

		if _, read := m.Update(tickOf(t, tick)); read != nil {
			t.Error("a tick after the close asked for a read")
		}
		if calls := historyCalls(history); len(calls) != 1 {
			t.Errorf("History read %v, want none after the close", calls)
		}
	})

	t.Run("stepping on stops the last session's refresh", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "hi")
		m, _ = pressStep(t, m)

		if _, read := m.Update(tickOf(t, tick)); read != nil {
			t.Error("the stepped-from session's tick asked for a read")
		}
		if calls := historyCalls(history); len(calls) != 1 {
			t.Errorf("History read %v, want none for the session stepped from", calls)
		}
	})

	t.Run("a read the close ends draws nothing and asks for no tick", func(t *testing.T) {
		m, _, tick := refreshingPreview(t, "hi")
		updated, read := m.Update(tickOf(t, tick))
		dismissPreview(t, asModel(t, updated))

		if msg := read(); msg != nil {
			t.Errorf("the read after the close answered %#v, want nothing", msg)
		}
	})

	t.Run("a session gone mid-watch says so, and the watch goes on", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, "hi")
		history.Remove("alpha")

		updated, read := m.Update(tickOf(t, tick))
		updated, next := asModel(t, updated).Update(read())

		if got := lastBodyRow(asModel(t, updated).preview); got != previewSessionGone {
			t.Errorf("body = %q, want %q", got, previewSessionGone)
		}
		tickOf(t, next)
	})
}

func TestPreviewRefresh_HoldsTheScroll(t *testing.T) {
	// lines is a history of n numbered lines, ending on its last.
	lines := func(n int) string {
		rows := make([]string, n)
		for i := range rows {
			rows[i] = fmt.Sprintf("line %d", i+1)
		}
		return strings.Join(rows, "\r\n")
	}
	refreshed := func(t *testing.T, m Model, history *backendtest.Fake, tick tea.Cmd, vt string) Model {
		t.Helper()
		history.SetHistory("alpha", []byte(vt))
		updated, read := m.Update(tickOf(t, tick))
		return asModel(t, dropCmd(asModel(t, updated).Update(read())))
	}

	t.Run("a preview scrolled up stays at the row the user moved to", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, lines(200))
		m, _ = pressKeys(m, tea.KeyPressMsg{Code: 'k', Text: "k"}, tea.KeyPressMsg{Code: 'k', Text: "k"}, tea.KeyPressMsg{Code: 'k', Text: "k"})
		offset := m.preview.viewport.YOffset()

		m = refreshed(t, m, history, tick, lines(205))

		if got := m.preview.viewport.YOffset(); got != offset {
			t.Errorf("offset = %d after the refresh, want the %d the user scrolled to", got, offset)
		}
	})

	t.Run("a preview at its bottom follows the session's new bottom", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, lines(200))

		m = refreshed(t, m, history, tick, lines(205))

		if !m.preview.viewport.AtBottom() || lastBodyRow(m.preview) != "line 205" {
			t.Errorf("at bottom %v, last row %q; want the new bottom", m.preview.viewport.AtBottom(), lastBodyRow(m.preview))
		}
	})
}
