package cmd

// Task spectrum-tui-design-5-6 — cmd-side fatal terminal event (§10.5 / §10.2).
//
// On the concurrent cold/TUI route a fatal bootstrap step makes the orchestrator
// goroutine's Run return a *bootstrap.FatalError. The pipe must map the terminal
// channel event onto a tui.BootstrapFatalMsg (NOT a BootstrapCompleteMsg) carrying
// the failed step index, the FatalError.UserMessage, and the underlying error —
// so the loading-page model enters the §10.5 error state and openTUI can extract
// the fatal for the non-zero exit. Per the 5-2 carry-forward, the failed step +
// message ride the terminal channel EVENT (value copies), not the pipe's struct
// fields, to avoid a happens-before race with the goroutine's writes.
//
// cmd rule: NO t.Parallel().

import (
	"context"
	"errors"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

// fatalAfterRunner emits `emitSteps` step-complete events (1..emitSteps) — the
// successful steps before the abort — then returns the fatal, mirroring the real
// orchestrator: a fatal step emits no event for itself, so the next un-emitted
// index (emitSteps+1) is the one that failed.
type fatalAfterRunner struct {
	emitSteps int
	started   bool
	err       error
}

func (r *fatalAfterRunner) Run(ctx context.Context) (bool, []bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	for i := 1; i <= r.emitSteps; i++ {
		if emit != nil {
			emit(bootstrap.StepEvent{Index: i, Name: "step"})
		}
	}
	return r.started, nil, r.err
}

// TestBootstrapProgressPipe_FatalMapsToFatalMsg asserts a fatal at step 3 makes
// the terminal channel event a tui.BootstrapFatalMsg carrying FailedStep=3, the
// FatalError.UserMessage, and the underlying error — and NOT a BootstrapCompleteMsg.
func TestBootstrapProgressPipe_FatalMapsToFatalMsg(t *testing.T) {
	cause := errors.New("permission denied")
	fatal := bootstrap.NewFatal("Portal failed to set @portal-restoring marker: permission denied", cause)
	// 2 steps emitted, then a fatal: the failed step is the next un-emitted step (3).
	runner := &fatalAfterRunner{emitSteps: 2, started: true, err: fatal}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var fatalMsg *tui.BootstrapFatalMsg
	for _, m := range msgs {
		if _, ok := m.(tui.BootstrapCompleteMsg); ok {
			t.Error("fatal run produced a BootstrapCompleteMsg; want a BootstrapFatalMsg")
		}
		if fm, ok := m.(tui.BootstrapFatalMsg); ok {
			fmCopy := fm
			fatalMsg = &fmCopy
		}
	}
	if fatalMsg == nil {
		t.Fatal("fatal run never produced a tui.BootstrapFatalMsg")
	}
	if fatalMsg.FailedStep != 3 {
		t.Errorf("BootstrapFatalMsg.FailedStep = %d, want 3 (the aborting step)", fatalMsg.FailedStep)
	}
	if fatalMsg.Message != fatal.UserMessage {
		t.Errorf("BootstrapFatalMsg.Message = %q, want %q (FatalError.UserMessage)", fatalMsg.Message, fatal.UserMessage)
	}
	if !errors.Is(fatalMsg.Err, cause) {
		t.Errorf("BootstrapFatalMsg.Err did not carry the fatal cause; got %v", fatalMsg.Err)
	}
	// The underlying *bootstrap.FatalError must be recoverable for the exit-code path.
	var asFatal *bootstrap.FatalError
	if !errors.As(fatalMsg.Err, &asFatal) {
		t.Error("BootstrapFatalMsg.Err is not a *bootstrap.FatalError (exit classification would miss it)")
	}
}

// TestBootstrapProgressPipe_FatalAtStep1 asserts a fatal before any step emits
// (EnsureServer) maps to FailedStep=1.
func TestBootstrapProgressPipe_FatalAtStep1(t *testing.T) {
	fatal := bootstrap.NewFatal("Portal failed to start tmux server: boom", errors.New("boom"))
	runner := &fatalAfterRunner{emitSteps: 0, started: false, err: fatal}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var fatalMsg *tui.BootstrapFatalMsg
	for _, m := range msgs {
		if fm, ok := m.(tui.BootstrapFatalMsg); ok {
			fmCopy := fm
			fatalMsg = &fmCopy
		}
	}
	if fatalMsg == nil {
		t.Fatal("fatal-at-step-1 run never produced a tui.BootstrapFatalMsg")
	}
	if fatalMsg.FailedStep != 1 {
		t.Errorf("BootstrapFatalMsg.FailedStep = %d, want 1 (EnsureServer)", fatalMsg.FailedStep)
	}
}

// TestBootstrapProgressPipe_NonFatalStillCompletes asserts a successful run still
// maps the terminal event to a BootstrapCompleteMsg (no fatal msg) — the fatal
// path must not leak into the success path.
func TestBootstrapProgressPipe_NonFatalStillCompletes(t *testing.T) {
	runner := &emittingRunner{steps: 11, started: true}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var sawComplete, sawFatal bool
	for _, m := range msgs {
		switch m.(type) {
		case tui.BootstrapCompleteMsg:
			sawComplete = true
		case tui.BootstrapFatalMsg:
			sawFatal = true
		}
	}
	if !sawComplete {
		t.Error("successful run did not produce a BootstrapCompleteMsg")
	}
	if sawFatal {
		t.Error("successful run produced a BootstrapFatalMsg; the fatal path leaked")
	}
}
