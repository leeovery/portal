package cmd

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/tui"
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

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				s.Add(bootstrap.Warning{Lines: []string{"x"}})
			}
		}()
	}
	drained := make(chan int, goroutines)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			drained <- len(s.Drain())
		}()
	}
	wg.Wait()
	close(drained)

	finalRemainder := len(s.Drain())

	total := finalRemainder
	for n := range drained {
		total += n
	}
	want := goroutines * perGoroutine
	if total != want {
		t.Errorf("total drained = %d, want %d (Add/Drain must not lose entries)", total, want)
	}
}

func TestStageBootstrapWarningsOnModel(t *testing.T) {
	t.Run("empty sink leaves model pending warnings nil", func(t *testing.T) {
		bootstrapWarnings = &BootstrapWarningsSink{}
		m := tui.New(fakePickerLister())

		stageBootstrapWarningsOnModel(&m)

		if got := m.PendingBootstrapWarnings(); got != nil {
			t.Errorf("PendingBootstrapWarnings = %#v, want nil", got)
		}
	})

	t.Run("non-empty sink stages converted warnings on model", func(t *testing.T) {
		bootstrapWarnings = &BootstrapWarningsSink{}
		bootstrapWarnings.Add(bootstrap.SaverDownWarning())
		bootstrapWarnings.Add(bootstrap.CorruptSessionsJSONWarning())
		m := tui.New(fakePickerLister())

		stageBootstrapWarningsOnModel(&m)

		got := m.PendingBootstrapWarnings()
		if len(got) != 2 {
			t.Fatalf("PendingBootstrapWarnings len = %d, want 2", len(got))
		}
		wantFirst := bootstrap.SaverDownWarning().Lines
		wantSecond := bootstrap.CorruptSessionsJSONWarning().Lines
		if !equalStringSlices(got[0].Lines, wantFirst) {
			t.Errorf("got[0].Lines = %v, want %v", got[0].Lines, wantFirst)
		}
		if !equalStringSlices(got[1].Lines, wantSecond) {
			t.Errorf("got[1].Lines = %v, want %v", got[1].Lines, wantSecond)
		}

		if remaining := bootstrapWarnings.Drain(); len(remaining) != 0 {
			t.Errorf("sink not drained; remaining = %d", len(remaining))
		}
	})
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPersistentPreRunE_EmitsWarningsToStderrOnCLIPath(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{
		started: false,
		warnings: []bootstrap.Warning{
			bootstrap.SaverDownWarning(),
			bootstrap.CorruptSessionsJSONWarning(),
		},
	}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

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
		"Run `portal doctor` for details.",
		"Portal state file unusable — restoration skipped.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("stderr missing line %q\nfull stderr:\n%s", want, got)
		}
	}

	saverIdx := strings.Index(got, "Portal save daemon failed")
	corruptIdx := strings.Index(got, "Portal state file unusable")
	if saverIdx < 0 || corruptIdx < 0 {
		t.Fatalf("expected both warnings in stderr; got %q", got)
	}
	if saverIdx >= corruptIdx {
		t.Errorf("saver warning must precede corrupt-index; saverIdx=%d, corruptIdx=%d", saverIdx, corruptIdx)
	}
}

func TestPersistentPreRunE_DoesNotEmitWarningsForOpenWithNoArgs(t *testing.T) {
	assertPickerPathWritesNoWarnings(t, "open")
}

// assertPickerPathWritesNoWarnings pins that a picker launch writes nothing to
// the terminal the picker claims: its bootstrap is deferred, and the warnings
// ride the progress pipe's terminal event to the notice band.
func assertPickerPathWritesNoWarnings(t *testing.T, args ...string) {
	t.Helper()
	resetBootstrapOnce(t)

	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error { return nil })
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

	runner := &recordingRunner{
		warnings: []bootstrap.Warning{bootstrap.SaverDownWarning()},
	}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	resetRootCmd()
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty on the picker path; got %q", stderr.String())
	}
	if runner.calls != 0 {
		t.Errorf("orchestrator calls = %d before the picker, want 0 (deferred)", runner.calls)
	}
}

func TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{
		warnings: []bootstrap.Warning{bootstrap.SaverDownWarning()},
	}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

	// Resolution fails on the non-existent path; warning emission happens before
	// RunE, so that failure is irrelevant here.
	resetRootCmd()
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"open", "/nonexistent/path-for-test"})
	_ = rootCmd.Execute()

	if !strings.Contains(stderr.String(), "Portal save daemon failed") {
		t.Errorf("stderr should contain SaverDownWarning on `open <path>` (CLI shape); got %q", stderr.String())
	}
}

func TestPersistentPreRunE_DoesNotEmitWarningsForOpenWithSearchForm(t *testing.T) {
	assertPickerPathWritesNoWarnings(t, "open", "/port")
}
