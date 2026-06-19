package tui

import (
	"io"
	"os"

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
