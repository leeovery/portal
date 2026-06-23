package tui

import (
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/portal/internal/warning"
)

// BootstrapWarning is a type alias for internal/warning.Warning. The
// alias keeps existing tui call sites readable while pointing at the
// canonical shape shared with cmd/bootstrap.
type BootstrapWarning = warning.Warning

// flushWarningsToStderr is the side-effect closure used by
// (Model).flushBufferedWarningsCmd to emit buffered warnings between
// alt-screen toggles. It is a package-level variable so tests can replace
// it with a recording stub via SetFlushWarningsToStderrForTest. The
// production implementation calls WriteBootstrapWarnings against
// os.Stderr.
var flushWarningsToStderr = func(warnings []BootstrapWarning) {
	WriteBootstrapWarnings(os.Stderr, warnings)
}

// WriteBootstrapWarnings emits every warning's lines to w in order, one
// Fprintln per line. Delegates to warning.WriteLines so the CLI and TUI
// paths share a single emission implementation. Errors from Fprintln are
// intentionally ignored — diagnostics must not themselves fail the
// program.
func WriteBootstrapWarnings(w io.Writer, warnings []BootstrapWarning) {
	warning.WriteLines(w, warnings)
}

// SetFlushWarningsToStderrForTest replaces the package-level
// flushWarningsToStderr seam with fn and returns a restore function. It
// is intended exclusively for tests in the tui_test package; production
// code MUST NOT call it.
func SetFlushWarningsToStderrForTest(fn func([]BootstrapWarning)) func() {
	prev := flushWarningsToStderr
	flushWarningsToStderr = fn
	return func() { flushWarningsToStderr = prev }
}

// formatWarningsFlash flattens soft bootstrap warnings into a single notice-band
// message: every warning's lines, in orchestrator-observation order, joined by
// newlines. The §11 notice band wraps and repeats its left-bar on every line, so a
// multi-line message renders as a multi-row band carrying every warning in order.
// Empty/nil input (and warnings with no lines) yield "" — the empty-string
// no-band sentinel the §11 single-slot arbiter treats as "show nothing", which is
// what preserves the zero-warnings → no-band / no-spurious-toggle property on the
// cold/TUI route.
func formatWarningsFlash(warnings []BootstrapWarning) string {
	var lines []string
	for _, w := range warnings {
		lines = append(lines, w.Lines...)
	}
	return strings.Join(lines, "\n")
}

// flushBufferedWarningsCmd returns a tea.Cmd that, when run by the Bubble
// Tea program, emits every buffered warning to stderr in order. Buffered
// warnings are cleared at the moment this method runs so repeat transitions
// do not re-emit the same lines.
//
// Bubble Tea v2 removed the imperative tea.ExitAltScreen / tea.EnterAltScreen
// commands (alt-screen is now a declarative tea.View field — set once in
// View). The v1 implementation wrapped the stderr write in an exit/enter
// alt-screen toggle so the lines surfaced in the terminal scrollback mid-run;
// under v2 that toggle is no longer expressible as a command, so the write
// stands alone and the warnings surface when the alt-screen is torn down on
// program exit. The emission itself (order, content, single-flush) is
// unchanged.
//
// Returns nil when no warnings are buffered — avoids dispatching a no-op cmd.
func (m *Model) flushBufferedWarningsCmd() tea.Cmd {
	if len(m.bufferedWarnings) == 0 {
		return nil
	}
	warnings := m.bufferedWarnings
	m.bufferedWarnings = nil
	return func() tea.Msg {
		flushWarningsToStderr(warnings)
		return nil
	}
}

// surfaceBufferedWarnings drains the warnings buffered through the loading window
// and surfaces them on the post-transition Sessions page, ROUTING by bootstrap
// path (§10.5):
//
//   - Concurrent cold/TUI route (progressReceiver != nil): the warnings rode the
//     progress channel onto the terminal BootstrapCompleteMsg while the TUI was
//     live, so the pre-launch package-sink staging never captured them. They are
//     surfaced IN-TUI as a §11 post-load notice band — a TRANSIENT orange/warning
//     flash routed through the single-slot arbiter (activeNoticeBand → bandWarning),
//     which auto-clears on the next actionable keypress via the §11.2
//     flashGen/isActionableKey lifecycle (the same transient hand-off a flash uses).
//     A 3 s auto-clear tick is scheduled too, exactly as the session-gone flash does,
//     so the band clears on keypress OR timeout. NO stderr / alt-screen flush on this
//     route — the in-TUI band replaces it. Zero warnings → no band
//     (formatWarningsFlash yields "", the arbiter's empty-string no-band sentinel),
//     and no tick is scheduled, preserving the no-spurious-toggle property.
//
//   - Synchronous warm/staging route (progressReceiver == nil): unchanged — the
//     warnings flush to stderr via flushBufferedWarningsCmd (the alt-screen-era
//     delivery), byte-for-byte as before.
//
// Called by the two transition sites (the LoadingMinElapsedMsg and
// BootstrapCompleteMsg arms) in place of the direct flushBufferedWarningsCmd call,
// so both gates route warnings identically.
func (m *Model) surfaceBufferedWarnings() tea.Cmd {
	if m.progressReceiver == nil {
		// Warm/staging route: unchanged stderr flush.
		return m.flushBufferedWarningsCmd()
	}
	// Cold/TUI route: surface as a transient in-TUI warning band, never stderr.
	message := formatWarningsFlash(m.bufferedWarnings)
	m.bufferedWarnings = nil
	if message == "" {
		// Zero warnings → no band, no tick (no spurious alt-screen toggle).
		return nil
	}
	m.setFlash(message)
	return flashTickCmd(m.flashGen)
}
