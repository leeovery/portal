package tui

import (
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

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
// Tea program, exits the alt-screen, emits every buffered warning to
// stderr in order, then re-enters the alt-screen. Buffered warnings are
// cleared at the moment this method runs so repeat transitions do not
// re-emit the same lines.
//
// Returns nil when no warnings are buffered — avoids a spurious
// alt-screen toggle that would briefly reveal the underlying terminal.
func (m *Model) flushBufferedWarningsCmd() tea.Cmd {
	if len(m.bufferedWarnings) == 0 {
		return nil
	}
	warnings := m.bufferedWarnings
	m.bufferedWarnings = nil
	return tea.Sequence(
		tea.ExitAltScreen,
		func() tea.Msg {
			flushWarningsToStderr(warnings)
			return nil
		},
		tea.EnterAltScreen,
	)
}
