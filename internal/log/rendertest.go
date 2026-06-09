package log

import (
	"log/slog"
	"testing"
	"time"
)

// Pinned baselines for the render seam. They are fixed (rather than derived from
// the live process) so a test fixture rendered via RenderLineForTest is
// deterministic across runs and machines — the baselines are present and
// well-formed (so the line round-trips through ParseLogLine) but never the
// assertion's subject. The values are deliberately single-token so they need no
// quoting.
const (
	testRenderPID         = 0
	testRenderVersion     = "test"
	testRenderProcessRole = "test"
)

// RenderLineForTest renders a single record to its canonical portal.log text
// line through the SAME production render path textHandler.Handle uses — never a
// re-implementation of the format. It is the only sanctioned way to obtain
// real-writer output in a test without driving log.Init: it constructs a
// textHandler via the production newTextHandler constructor, builds a slog.Record
// from the caller-supplied time/level/message/attrs (with component supplied as
// the component attr so the existing component-resolution path renders it as the
// literal prefix), and returns h.render(record) — the line Handle would have
// written, including the trailing newline.
//
// The caller-supplied ts is rendered through the same r.Time.Format(RFC3339Nano)
// path Handle uses, so the fixture's timestamp is exactly what ParseLogLine
// expects.
//
// It performs NO process-global handler mutation (no setHandler/Init/
// SetTestHandler) and NO sink write — it is side-effect-free. Production code
// must never call it: the *testing.T-first parameter structurally marks it
// test-only (it cannot be referenced from non-test code), mirroring
// SetTestHandler and portaltest.IsolateStateForTest.
func RenderLineForTest(t *testing.T, ts time.Time, level slog.Level, component, message string, attrs ...slog.Attr) string {
	t.Helper()

	h := newTextHandler(nil, level, testRenderPID, testRenderVersion, testRenderProcessRole).(*textHandler)
	h = h.WithAttrs([]slog.Attr{slog.String(componentKey, component)}).(*textHandler)

	r := slog.NewRecord(ts, level, message, 0)
	r.AddAttrs(attrs...)

	return h.render(r)
}
