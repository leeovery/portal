package cmd

// Tests that drive BootstrapWarningsSink mutate package-level state and
// MUST NOT use t.Parallel.

import (
	"fmt"
	"io"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
)

// BootstrapWarningsSink accumulates soft bootstrap warnings emitted by
// the orchestrator. Two consumers drain it:
//
//   - The CLI path (PersistentPreRunE for non-TUI commands) calls EmitTo
//     to flush every buffered line to stderr before RunE executes.
//   - The TUI path (openTUI; Phase 6 task 6-10) drains the sink AFTER the
//     loading page dismisses so direct stderr writes do not corrupt the
//     Bubble Tea alt-screen rendering.
//
// All operations are safe under concurrent use; the orchestrator runs in
// PersistentPreRunE on the main goroutine but consumers may drain from
// other goroutines (Bubble Tea runs Update/View off the main goroutine
// in some flows).
type BootstrapWarningsSink struct {
	mu       sync.Mutex
	warnings []bootstrap.Warning
}

// Add appends a single warning to the sink. Safe for concurrent use.
func (s *BootstrapWarningsSink) Add(w bootstrap.Warning) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, w)
}

// Drain returns every buffered warning and clears the sink atomically.
// Safe for concurrent use; concurrent callers receive disjoint slices.
// Returns a nil slice when the sink is empty.
func (s *BootstrapWarningsSink) Drain() []bootstrap.Warning {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.warnings
	s.warnings = nil
	return out
}

// EmitTo drains the sink and writes every warning's lines to w in
// orchestrator-observation order, one line per Fprintln. Safe for
// concurrent use — Drain's atomicity guarantees no warning is emitted
// twice across concurrent EmitTo callers.
func (s *BootstrapWarningsSink) EmitTo(w io.Writer) {
	for _, warn := range s.Drain() {
		for _, line := range warn.Lines {
			_, _ = fmt.Fprintln(w, line)
		}
	}
}

// bootstrapWarnings is the canonical package-level sink. PersistentPreRunE
// adds to it after the orchestrator returns; openTUI drains it after
// loading-page dismissal (task 6-10), and PersistentPreRunE drains it
// directly to stderr for non-TUI commands.
var bootstrapWarnings = &BootstrapWarningsSink{}
