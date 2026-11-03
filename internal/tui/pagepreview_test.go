package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// historyFake holds each named session with vt as its history.
func historyFake(t *testing.T, vt string, names ...string) *backendtest.Fake {
	t.Helper()
	f := backendtest.New()
	for _, name := range names {
		f.Add(backend.Session{Name: name})
		f.SetHistory(name, []byte(vt))
	}
	return f
}

// newTestPreview opens a preview of session "work" whose history is vt.
func newTestPreview(t *testing.T, vt string, width, height int) previewModel {
	t.Helper()
	m, ok := NewPreviewModel("work", newPreviewHistory(historyFake(t, vt, "work")), width, height)
	if !ok {
		t.Fatalf("setup: NewPreviewModel returned ok=false")
	}
	return m
}

// previewBody is the viewport's rows as text, right-trimmed, without the blank
// rows that pad a short history to the viewport's height.
func previewBody(m previewModel) []string {
	rows := strings.Split(stripANSI(m.viewport.View()), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	for len(rows) > 1 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// stallingHistory answers nothing until its context ends, then fails with the
// context's cause, as a wedged session does under the Backend contract.
type stallingHistory struct {
	deadline, entered time.Time
}

func (s *stallingHistory) History(ctx context.Context, _ string, _ int) ([]byte, error) {
	s.entered = time.Now()
	s.deadline, _ = ctx.Deadline()
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func TestNewPreviewModel_BodyIsTheSessionsWholeHistory(t *testing.T) {
	fake := historyFake(t, "first\r\nsecond", "work")

	m, ok := NewPreviewModel("work", newPreviewHistory(fake), 80, 24)

	if !ok {
		t.Fatalf("NewPreviewModel ok=false, want true")
	}
	calls := fake.CallsTo(backendtest.OpHistory)
	if len(calls) != 1 || calls[0].Name != "work" || calls[0].Rows != 0 {
		t.Errorf("History calls = %+v, want one for work with rows 0 (the whole history)", calls)
	}
	if got := previewBody(m); !slices.Equal(got[len(got)-2:], []string{"first", "second"}) {
		t.Errorf("body = %q, want it to end with the history's two rows", got)
	}
}

func TestNewPreviewModel_DrawsTheHistoryAsTheTerminalWould(t *testing.T) {
	// zmx's own serialisation: a CUF step, a background erase, then the
	// cursor's CUP and the state after it.
	m := newTestPreview(t, "a\x1b[3Cb\r\n\x1b[44mx\x1b[2X\x1b[2C\x1b[0m\x1b[2;4H\x1b[0m", 80, 24)

	body := previewBody(m)
	if got := body[len(body)-2:]; !slices.Equal(got, []string{"a   b", "x"}) {
		t.Errorf("body = %q, want the CUF drawn as blanks and nothing after the CUP", got)
	}
	if view := m.viewport.View(); strings.Contains(view, "\x1b[2X") || strings.Contains(view, "\x1b[2;4H") {
		t.Errorf("viewport content carries a non-SGR sequence: %q", view)
	}
}

func TestNewPreviewModel_OpensAtTheBottomOfTheHistory(t *testing.T) {
	m := newTestPreview(t, strings.Repeat("line\r\n", 50)+"last", 80, 24)

	if !m.viewport.AtBottom() {
		t.Errorf("viewport.AtBottom() = false right after construction, want true")
	}
}

func TestNewPreviewModel_SaysWhenTheSessionIsNotAnswering(t *testing.T) {
	fake := historyFake(t, "unread", "work")
	fake.StopAnswering("work")

	m, ok := NewPreviewModel("work", newPreviewHistory(fake), 80, 24)

	if !ok {
		t.Fatalf("NewPreviewModel ok=false, want the preview to open and say why it is empty")
	}
	if got := previewBody(m); got[len(got)-1] != previewNotAnswering {
		t.Errorf("body = %q, want %q", got, previewNotAnswering)
	}
}

func TestNewPreviewModel_SaysWhenTheHistoryIsUnavailable(t *testing.T) {
	for name, fail := range map[string]error{
		"unavailable":     backend.ErrHistoryUnavailable,
		"any other error": errors.New("connection reset"),
	} {
		t.Run(name, func(t *testing.T) {
			fake := historyFake(t, "unread", "work")
			fake.Fail(backendtest.OpHistory, fail)

			m, ok := NewPreviewModel("work", newPreviewHistory(fake), 80, 24)

			if !ok {
				t.Fatalf("NewPreviewModel ok=false, want the preview to open and say why it is empty")
			}
			if got := previewBody(m); got[len(got)-1] != previewHistoryUnavailable {
				t.Errorf("body = %q, want %q", got, previewHistoryUnavailable)
			}
		})
	}
}

func TestNewPreviewModel_OpensNothingForAGoneSession(t *testing.T) {
	fake := historyFake(t, "x", "other")

	if _, ok := NewPreviewModel("work", newPreviewHistory(fake), 80, 24); ok {
		t.Errorf("NewPreviewModel ok=true for a session the backend no longer holds, want false")
	}
}

func TestNewPreviewModel_OpensNothingWithoutAHistoryReader(t *testing.T) {
	if _, ok := NewPreviewModel("work", previewHistory{}, 80, 24); ok {
		t.Errorf("NewPreviewModel ok=true with no history reader, want false")
	}
}

func TestNewPreviewModel_AWedgedSessionHoldsThePreviewOnlyToTheCeiling(t *testing.T) {
	const ceiling = 20 * time.Millisecond
	stall := &stallingHistory{}
	start := time.Now()

	m, ok := NewPreviewModel("work", previewHistory{reader: stall, ceiling: ceiling}, 80, 24)

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("NewPreviewModel took %s against a %s ceiling", elapsed, ceiling)
	}
	if !ok {
		t.Fatalf("NewPreviewModel ok=false, want the preview to open")
	}
	if stall.deadline.IsZero() || stall.deadline.Sub(stall.entered) > ceiling {
		t.Errorf("History's context deadline = %v, want one within %s of the call", stall.deadline, ceiling)
	}
	if got := previewBody(m); got[len(got)-1] != previewNotAnswering {
		t.Errorf("body = %q, want %q once the ceiling passes", got, previewNotAnswering)
	}
}

func TestWithHistory_BoundsEveryReadByTheCeiling(t *testing.T) {
	m := New(nil, WithHistory(historyFake(t, "x", "work")))

	if m.history.ceiling != previewHistoryCeiling {
		t.Errorf("history ceiling = %s, want %s", m.history.ceiling, previewHistoryCeiling)
	}
}

func TestNewPreviewModel_ReadsAfreshEachTimeItOpens(t *testing.T) {
	fake := historyFake(t, "first", "work")
	history := newPreviewHistory(fake)

	if _, ok := NewPreviewModel("work", history, 80, 24); !ok {
		t.Fatalf("first open: ok=false")
	}
	fake.SetHistory("work", []byte("second"))
	m, ok := NewPreviewModel("work", history, 80, 24)
	if !ok {
		t.Fatalf("second open: ok=false")
	}

	if n := len(fake.CallsTo(backendtest.OpHistory)); n != 2 {
		t.Errorf("History calls = %d, want one per open", n)
	}
	if got := previewBody(m); got[len(got)-1] != "second" {
		t.Errorf("second open's body = %q, want the history as it is now", got)
	}
}
