package cmd

// Tests in this file mutate the package-level bootstrapWarnings sink and
// MUST NOT use t.Parallel.

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/spf13/cobra"
)

func TestBootstrapWarningsSink_AddBuffersWarning(t *testing.T) {
	s := &BootstrapWarningsSink{}
	w := bootstrap.Warning{Lines: []string{"line one", "line two"}}

	s.Add(w)

	got := s.Drain()
	if len(got) != 1 {
		t.Fatalf("Drain len = %d, want 1", len(got))
	}
	if len(got[0].Lines) != 2 || got[0].Lines[0] != "line one" || got[0].Lines[1] != "line two" {
		t.Errorf("Drain returned %#v, want lines [line one, line two]", got[0].Lines)
	}
}

func TestBootstrapWarningsSink_DrainClearsBuffer(t *testing.T) {
	s := &BootstrapWarningsSink{}
	s.Add(bootstrap.Warning{Lines: []string{"x"}})

	if got := len(s.Drain()); got != 1 {
		t.Fatalf("first Drain len = %d, want 1", got)
	}
	if got := len(s.Drain()); got != 0 {
		t.Errorf("second Drain len = %d, want 0 (buffer must clear atomically)", got)
	}
}

func TestBootstrapWarningsSink_DrainEmptySinkReturnsNil(t *testing.T) {
	s := &BootstrapWarningsSink{}

	got := s.Drain()
	if got != nil {
		t.Errorf("Drain on empty sink = %#v, want nil", got)
	}
}

func TestBootstrapWarningsSink_EmitToWritesEachLineInOrder(t *testing.T) {
	s := &BootstrapWarningsSink{}
	s.Add(bootstrap.Warning{Lines: []string{"first warn line 1", "first warn line 2"}})
	s.Add(bootstrap.Warning{Lines: []string{"second warn line 1"}})

	var buf bytes.Buffer
	s.EmitTo(&buf)

	want := "first warn line 1\nfirst warn line 2\nsecond warn line 1\n"
	if buf.String() != want {
		t.Errorf("EmitTo wrote %q; want %q", buf.String(), want)
	}
}

func TestBootstrapWarningsSink_EmitToDrainsBuffer(t *testing.T) {
	s := &BootstrapWarningsSink{}
	s.Add(bootstrap.Warning{Lines: []string{"x"}})

	var buf bytes.Buffer
	s.EmitTo(&buf)

	if got := len(s.Drain()); got != 0 {
		t.Errorf("Drain after EmitTo len = %d, want 0", got)
	}
}

func TestBootstrapWarningsSink_EmitToOnEmptySinkIsNoOp(t *testing.T) {
	s := &BootstrapWarningsSink{}

	var buf bytes.Buffer
	s.EmitTo(&buf)

	if buf.Len() != 0 {
		t.Errorf("EmitTo on empty sink wrote %q; want empty", buf.String())
	}
}

func TestBootstrapWarningsSink_ConcurrentAddAndDrainAreSafe(t *testing.T) {
	s := &BootstrapWarningsSink{}

	const goroutines = 16
	const perGoroutine = 64
	var wg sync.WaitGroup

	// Concurrent Adds.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				s.Add(bootstrap.Warning{Lines: []string{"x"}})
			}
		}()
	}
	// Concurrent Drains, racing with Adds.
	drained := make(chan int, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			drained <- len(s.Drain())
		}()
	}
	wg.Wait()
	close(drained)

	// Final drain to flush whatever's left.
	finalRemainder := len(s.Drain())

	// Sum of every observed Drain plus the final remainder must equal the
	// total Adds. Disjoint slices are the contract.
	total := finalRemainder
	for n := range drained {
		total += n
	}
	want := goroutines * perGoroutine
	if total != want {
		t.Errorf("total drained = %d, want %d (Add/Drain must not lose entries)", total, want)
	}
}

// TestPersistentPreRunE_EmitsWarningsToStderrOnCLIPath verifies that for
// non-TUI commands, PersistentPreRunE drains every accumulated warning to
// stderr in orchestrator-observation order before the command's RunE
// executes.
func TestPersistentPreRunE_EmitsWarningsToStderrOnCLIPath(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{
		started: false,
		warnings: []bootstrap.Warning{
			bootstrap.SaverDownWarning(),
			bootstrap.CorruptSessionsJSONWarning(),
		},
	}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	listDeps = &ListDeps{
		Lister: &mockSessionLister{sessions: nil},
		IsTTY:  func() bool { return false },
	}
	t.Cleanup(func() { listDeps = nil })

	resetRootCmd()
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := stderr.String()
	wantLines := []string{
		"Portal save daemon failed to start — sessions won't be captured.",
		"Run `portal state status` for details.",
		"Portal state file is corrupt — restoration skipped.",
		"Check `portal state status` or ~/.config/portal/state/portal.log.",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("stderr missing line %q\nfull stderr:\n%s", want, got)
		}
	}

	// Order: saver lines must precede corrupt-index lines.
	saverIdx := strings.Index(got, "Portal save daemon failed")
	corruptIdx := strings.Index(got, "Portal state file is corrupt")
	if saverIdx < 0 || corruptIdx < 0 {
		t.Fatalf("expected both warnings in stderr; got %q", got)
	}
	if saverIdx >= corruptIdx {
		t.Errorf("saver warning must precede corrupt-index; saverIdx=%d, corruptIdx=%d", saverIdx, corruptIdx)
	}
}

// TestPersistentPreRunE_DoesNotEmitWarningsForOpenWithNoArgs verifies the
// TUI path: when invoked as `portal open` with zero positional args, the
// warnings stay buffered in the sink so openTUI (Phase 6 task 6-10) can
// drain them post-loading-page dismissal.
func TestPersistentPreRunE_DoesNotEmitWarningsForOpenWithNoArgs(t *testing.T) {
	resetBootstrapOnce(t)

	// Stub openTUIFunc so the TUI program never actually launches; we only
	// care about PersistentPreRunE's stderr behaviour up to that point.
	originalOpenTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error { return nil }
	t.Cleanup(func() { openTUIFunc = originalOpenTUI })

	runner := &recordingRunner{
		warnings: []bootstrap.Warning{bootstrap.SaverDownWarning()},
	}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty on TUI path (warnings buffered for openTUI); got %q", stderr.String())
	}

	// Sink retains the warning for openTUI to drain.
	remaining := bootstrapWarnings.Drain()
	if len(remaining) != 1 {
		t.Errorf("sink remaining warnings = %d, want 1 (still buffered for TUI)", len(remaining))
	}
}

// TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg verifies
// that `portal open <path>` is a CLI-shape invocation: PersistentPreRunE
// must emit warnings to stderr before openPath runs.
func TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{
		warnings: []bootstrap.Warning{bootstrap.SaverDownWarning()},
	}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	// Resolution will fail on a non-existent path, but PersistentPreRunE's
	// warning emission happens BEFORE RunE, so the resolution failure is
	// irrelevant to this assertion.
	resetRootCmd()
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"open", "/nonexistent-path-for-test"})
	_ = rootCmd.Execute()

	if !strings.Contains(stderr.String(), "Portal save daemon failed") {
		t.Errorf("stderr should contain SaverDownWarning on `open <path>` (CLI shape); got %q", stderr.String())
	}
}
