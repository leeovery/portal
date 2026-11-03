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

// openedPreview is the preview of session once its read from history has
// landed, as the picker shows it a moment after Space.
func openedPreview(t *testing.T, session string, history HistoryReader, width, height int) previewModel {
	t.Helper()
	return NewPreviewModel(Session{Name: session}, bodyOf(t, newPreviewHistory(history), session), width, height)
}

// bodyOf is h's body for session, read for an open that never ends.
func bodyOf(t *testing.T, h previewHistory, session string) string {
	t.Helper()
	body, ok := h.body(context.Background(), session)
	if !ok {
		t.Fatalf("body(%q) ok=false for an open that never ended", session)
	}
	return body
}

// newTestPreview is the opened preview of session "work" whose history is vt.
func newTestPreview(t *testing.T, vt string, width, height int) previewModel {
	t.Helper()
	return openedPreview(t, "work", historyFake(t, vt, "work"), width, height)
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

func lastBodyRow(m previewModel) string {
	body := previewBody(m)
	return body[len(body)-1]
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

func TestPreviewHistoryBody_IsTheSessionsWholeHistory(t *testing.T) {
	fake := historyFake(t, "first\r\nsecond", "work")

	body := bodyOf(t, newPreviewHistory(fake), "work")

	calls := fake.CallsTo(backendtest.OpHistory)
	if len(calls) != 1 || calls[0].Name != "work" || calls[0].Rows != 0 {
		t.Errorf("History calls = %+v, want one for work with rows 0 (the whole history)", calls)
	}
	if got := stripANSI(body); got != "first\nsecond" {
		t.Errorf("body = %q, want the history's two rows", got)
	}
}

func TestPreviewHistoryBody_DrawsTheHistoryAsTheTerminalWould(t *testing.T) {
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

func TestPreviewHistoryBody_SaysWhyThereIsNone(t *testing.T) {
	for name, tc := range map[string]struct {
		stage func(*backendtest.Fake)
		want  string
	}{
		"not answering":   {func(f *backendtest.Fake) { f.StopAnswering("work") }, previewNotAnswering},
		"unavailable":     {func(f *backendtest.Fake) { f.Fail(backendtest.OpHistory, backend.ErrHistoryUnavailable) }, previewHistoryUnavailable},
		"any other error": {func(f *backendtest.Fake) { f.Fail(backendtest.OpHistory, errors.New("connection reset")) }, previewHistoryUnavailable},
		"gone":            {func(f *backendtest.Fake) { f.Remove("work") }, previewSessionGone},
	} {
		t.Run(name, func(t *testing.T) {
			fake := historyFake(t, "unread", "work")
			tc.stage(fake)

			if got := bodyOf(t, newPreviewHistory(fake), "work"); got != tc.want {
				t.Errorf("body = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewHistoryBody_AWedgedSessionHoldsTheReadOnlyToTheCeiling(t *testing.T) {
	const ceiling = 20 * time.Millisecond
	stall := &stallingHistory{}
	start := time.Now()

	body := bodyOf(t, previewHistory{reader: stall, ceiling: ceiling}, "work")

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the read took %s against a %s ceiling", elapsed, ceiling)
	}
	if stall.deadline.IsZero() || stall.deadline.Sub(stall.entered) > ceiling {
		t.Errorf("History's context deadline = %v, want one within %s of the call", stall.deadline, ceiling)
	}
	if body != previewNotAnswering {
		t.Errorf("body = %q, want %q once the ceiling passes", body, previewNotAnswering)
	}
}

func TestPreviewHistoryRead_CarriesItsGeneration(t *testing.T) {
	msg := newPreviewHistory(historyFake(t, "x", "work")).read(context.Background(), "work", 7)()

	got, ok := msg.(previewBodyMsg)
	if !ok || got.gen != 7 || got.body != "x" {
		t.Errorf("read's message = %#v, want the body for generation 7", msg)
	}
}

func TestPreviewHistoryRead_OfAnEndedOpenNeitherDialsNorDraws(t *testing.T) {
	fake := historyFake(t, "x", "work")
	open, end := context.WithCancel(context.Background())
	end()

	msg := newPreviewHistory(fake).read(open, "work", 1)()

	if msg != nil {
		t.Errorf("read of an ended open returned %#v, want no message", msg)
	}
	if n := len(fake.CallsTo(backendtest.OpHistory)); n != 0 {
		t.Errorf("History dialled %d times for an ended open, want none", n)
	}
}

// endingHistory answers with history, but ends the open it is read for first,
// as the next open does while a read is under way.
type endingHistory struct {
	end context.CancelFunc
}

func (h endingHistory) History(context.Context, string, int) ([]byte, error) {
	h.end()
	return []byte("drawn only if the read was still wanted"), nil
}

func TestPreviewHistoryRead_AnOpenEndedDuringTheReadDrawsNothing(t *testing.T) {
	open, end := context.WithCancel(context.Background())

	if msg := newPreviewHistory(endingHistory{end: end}).read(open, "work", 1)(); msg != nil {
		t.Errorf("read whose open ended mid-read returned %#v, want no message", msg)
	}
}

func TestWithHistory_BoundsEveryReadByTheCeiling(t *testing.T) {
	m := New(nil, WithHistory(historyFake(t, "x", "work")))

	if m.history.ceiling != previewHistoryCeiling {
		t.Errorf("history ceiling = %s, want %s", m.history.ceiling, previewHistoryCeiling)
	}
}

func TestNewPreviewModel_OpensAtTheBottomOfItsBody(t *testing.T) {
	m := newTestPreview(t, strings.Repeat("line\r\n", 50)+"last", 80, 24)

	if !m.viewport.AtBottom() {
		t.Errorf("viewport.AtBottom() = false right after construction, want true")
	}
}

func TestPreviewSetBody_ReplacesTheBodyAnchoredAtTheBottom(t *testing.T) {
	m := NewPreviewModel(Session{Name: "work"}, previewLoading, 80, 24)

	m.setBody(renderHistory([]byte(strings.Repeat("line\r\n", 50) + "last")))

	if got := lastBodyRow(m); got != "last" {
		t.Errorf("last row = %q, want the new body's", got)
	}
	if !m.viewport.AtBottom() {
		t.Errorf("viewport.AtBottom() = false after the body landed, want true")
	}
}
