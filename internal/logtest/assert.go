package logtest

import (
	"errors"
	"log/slog"

	"github.com/leeovery/portal/internal/harnesstest"
)

// RecordWant is the shape every audit-trail record shares: its level, its
// message, the component it was emitted under, and the op and via attrs. The
// message and the op attr are separate fields because they are separate parts
// of the line's contract, even where an emission sets both to the same string.
type RecordWant struct {
	Level     slog.Level
	Msg       string
	Component string
	Op        string
	Via       string
}

// AssertRecord checks the five shared properties, reporting each mismatch
// separately. The attrs that belong to one emission — a stand-down's reason, a
// failure's error — stay with their own caller.
func AssertRecord(t harnesstest.TestingT, rec Record, want RecordWant) {
	t.Helper()
	if rec.Level != want.Level {
		t.Errorf("level = %v, want %v", rec.Level, want.Level)
	}
	if rec.Msg != want.Msg {
		t.Errorf("message = %q, want %q", rec.Msg, want.Msg)
	}
	if got := rec.AttrString(t, "component"); got != want.Component {
		t.Errorf("component = %q, want %q", got, want.Component)
	}
	if got := rec.AttrString(t, "op"); got != want.Op {
		t.Errorf("op = %q, want %q", got, want.Op)
	}
	if got := rec.AttrString(t, "via"); got != want.Via {
		t.Errorf("via = %q, want %q", got, want.Via)
	}
}

// AssertWriteFailure checks the tail every failed-write breadcrumb carries: the
// error_class token the write phase classified to, and that the error attr
// carries a value wrapping sentinel. The caller names the sentinel rather than
// this helper resolving it, so the edge to the package declaring it stays with
// that caller instead of arriving in every test package in the tree, all of
// which may reach for this one.
func AssertWriteFailure(t harnesstest.TestingT, rec Record, wantClass string, sentinel error) {
	t.Helper()
	if got := rec.AttrString(t, "error_class"); got != wantClass {
		t.Errorf("error_class = %q, want %q", got, wantClass)
	}
	if err := rec.ErrorAttr(t, "error"); !errors.Is(err, sentinel) {
		t.Errorf("error attr does not wrap %v: %v", sentinel, err)
	}
}
