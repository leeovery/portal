package tui

import (
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/portal/internal/warning"
)

type BootstrapWarning = warning.Warning

// WriteBootstrapWarnings ignores write errors — diagnostics must not
// themselves fail the program.
func WriteBootstrapWarnings(w io.Writer, warnings []BootstrapWarning) {
	warning.WriteLines(w, warnings)
}

func formatWarningsFlash(warnings []BootstrapWarning) string {
	var lines []string
	for _, w := range warnings {
		lines = append(lines, w.Lines...)
	}
	return strings.Join(lines, "\n")
}

// The loading page surfaces warnings as an in-TUI band — a stderr flush
// mid-run would be lost under the live alt-screen.
func (m *Model) surfaceBufferedWarnings() tea.Cmd {
	message := formatWarningsFlash(m.bufferedWarnings)
	m.bufferedWarnings = nil
	if message == "" {
		return nil
	}
	m.setFlash(message)
	return flashTickCmd(m.flashGen)
}
