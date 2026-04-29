// Package warning provides the canonical shape and stderr emission helper
// for soft bootstrap warnings shared across the cmd and tui packages.
//
// Two consumers historically owned byte-identical copies of the same
// struct and emission loop sitting either side of the cmd→tui import
// boundary (cmd imports tui, so tui cannot import cmd). Hoisting the
// shape and writer here keeps a single source of truth: cmd/bootstrap
// aliases Warning and both emission sites delegate to WriteLines so the
// CLI and TUI paths produce identical stderr output for the same inputs.
package warning

import (
	"fmt"
	"io"
)

// Warning is a soft bootstrap failure that must NOT terminate Portal.
// The orchestrator accumulates Warnings during Run; the CLI path emits
// each warning's lines to stderr before returning from PersistentPreRunE
// while the TUI path buffers them and flushes after the loading page
// dismisses (see resurrection spec, Observability → Proactive Health
// Signals → TUI interaction).
//
// Lines are emitted in slice order, one line per Fprintln. No banners,
// no colors, no prefixes — the spec mandates a single primary line plus
// an optional follow-up pointer per warning.
type Warning struct {
	Lines []string
}

// WriteLines emits every warning's lines to w in slice order, one
// Fprintln per line. Errors from Fprintln are intentionally ignored —
// diagnostics must not themselves fail the program. The CLI path
// (cmd.BootstrapWarningsSink.EmitTo) and the TUI path
// (tui.WriteBootstrapWarnings) both delegate here so they produce
// byte-identical stderr output for the same inputs.
func WriteLines(w io.Writer, ws []Warning) {
	for _, warn := range ws {
		for _, line := range warn.Lines {
			_, _ = fmt.Fprintln(w, line)
		}
	}
}
