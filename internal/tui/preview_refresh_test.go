package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backend"
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

// numbered is a history of the numbered lines from first to last.
func numbered(first, last int) string {
	rows := make([]string, 0, last-first+1)
	for i := first; i <= last; i++ {
		rows = append(rows, fmt.Sprintf("line %d", i))
	}
	return strings.Join(rows, "\r\n")
}

// refreshed is m once its tick's read lands, the session's history now vt.
func refreshed(t *testing.T, m Model, history *backendtest.Fake, tick tea.Cmd, vt string) (Model, tea.Cmd) {
	t.Helper()
	history.SetHistory("alpha", []byte(vt))
	updated, read := m.Update(tickOf(t, tick))
	updated, next := asModel(t, updated).Update(read())
	return asModel(t, updated), next
}

// scrolledUp is m with its preview scrolled up n rows.
func scrolledUp(m Model, n int) Model {
	for range n {
		m, _ = pressKeys(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	}
	return m
}

// topRow is the row at the top of the preview's view.
func topRow(m Model) string {
	return previewBody(m.preview)[0]
}

func TestPreviewRefresh_HoldsTheScroll(t *testing.T) {
	t.Run("a preview scrolled up stays on the rows the user moved to", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, numbered(1, 200))
		m = scrolledUp(m, 3)
		top, offset := topRow(m), m.preview.viewport.YOffset()

		m, _ = refreshed(t, m, history, tick, numbered(1, 205))

		if got := m.preview.viewport.YOffset(); got != offset || topRow(m) != top {
			t.Errorf("offset %d showing %q after the refresh, want %d showing %q", got, topRow(m), offset, top)
		}
	})

	t.Run("a history at its cap, losing rows from its top as it grows, keeps the same rows in view", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, numbered(1, 200))
		m = scrolledUp(m, 3)
		top := topRow(m)

		m, _ = refreshed(t, m, history, tick, numbered(6, 205))

		if got := topRow(m); got != top {
			t.Errorf("the view shows %q at its top after the refresh, want the %q the user was reading", got, top)
		}
	})

	t.Run("rows no longer in the history leave the view at the row it was", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, numbered(1, 200))
		m = scrolledUp(m, 3)
		offset := m.preview.viewport.YOffset()

		m, _ = refreshed(t, m, history, tick, strings.ReplaceAll(numbered(1, 200), "line", "row"))

		if got := m.preview.viewport.YOffset(); got != offset {
			t.Errorf("offset = %d, want the %d it was", got, offset)
		}
	})

	t.Run("a preview at its bottom follows the session's new bottom", func(t *testing.T) {
		m, history, tick := refreshingPreview(t, numbered(1, 200))

		m, _ = refreshed(t, m, history, tick, numbered(1, 205))

		if !m.preview.viewport.AtBottom() || lastBodyRow(m.preview) != "line 205" {
			t.Errorf("at bottom %v, last row %q; want the new bottom", m.preview.viewport.AtBottom(), lastBodyRow(m.preview))
		}
	})
}

func TestPreviewRefresh_FailedReads(t *testing.T) {
	for _, err := range []error{backend.ErrTimeout, backend.ErrHistoryUnavailable} {
		t.Run("a read failing with "+err.Error()+" keeps the body and the place, and the refresh goes on", func(t *testing.T) {
			m, history, tick := refreshingPreview(t, numbered(1, 200))
			m = scrolledUp(m, 3)
			top, offset := topRow(m), m.preview.viewport.YOffset()
			history.Fail(backendtest.OpHistory, err)

			m, next := refreshed(t, m, history, tick, numbered(1, 205))

			if got := m.preview.viewport.YOffset(); got != offset || topRow(m) != top {
				t.Errorf("offset %d showing %q, want the body kept at %d showing %q", got, topRow(m), offset, top)
			}
			tickOf(t, next)
		})
	}

	t.Run("a read failing before any body came says why", func(t *testing.T) {
		sessions := []Session{{Name: "alpha"}}
		history := historyFor(t, "hi", sessions)
		history.Fail(backendtest.OpHistory, backend.ErrTimeout)
		m := modelWithSeams(t, sessions, history)
		m.previewRefresh = refreshEvery

		m, read := pressKeys(m, keySpaceMsg())
		m = landRead(t, m, read)

		if got := lastBodyRow(m.preview); got != previewNotAnswering {
			t.Errorf("body = %q, want %q", got, previewNotAnswering)
		}
	})

	t.Run("a body after failed reads replaces the line saying why", func(t *testing.T) {
		sessions := []Session{{Name: "alpha"}}
		history := historyFor(t, "hi", sessions)
		history.Fail(backendtest.OpHistory, backend.ErrTimeout)
		m := modelWithSeams(t, sessions, history)
		m.previewRefresh = refreshEvery
		m, read := pressKeys(m, keySpaceMsg())
		updated, tick := m.Update(read())
		history.Fail(backendtest.OpHistory, nil)

		m, _ = refreshed(t, asModel(t, updated), history, tick, "back")

		if got := lastBodyRow(m.preview); got != "back" {
			t.Errorf("body = %q, want the session's own", got)
		}
	})
}

func TestPreviewRefresh_ASessionListedNotAnswering(t *testing.T) {
	t.Run("ticks with nothing dialled, and comes alive once it answers", func(t *testing.T) {
		sessions := []Session{{Name: "alpha", NotAnswering: true}}
		history := historyFor(t, "awake", sessions)
		m := modelWithSeams(t, sessions, history)
		m.previewRefresh = refreshEvery

		m, tick := pressKeys(m, keySpaceMsg())
		if got := lastBodyRow(m.preview); got != previewNotAnswering {
			t.Fatalf("body = %q at once, want %q", got, previewNotAnswering)
		}
		if calls := historyCalls(history); len(calls) != 0 {
			t.Errorf("History read %v as the preview opened, want none", calls)
		}

		updated, read := m.Update(tickOf(t, tick))
		m = landRead(t, asModel(t, updated), read)

		if got := lastBodyRow(m.preview); got != "awake" {
			t.Errorf("body = %q after the tick, want the session's own", got)
		}
	})
}
