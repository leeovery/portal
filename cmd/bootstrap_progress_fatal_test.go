package cmd

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

type fatalAfterRunner struct {
	emitSteps int
	err       error
}

func (r *fatalAfterRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	for i := 1; i <= r.emitSteps; i++ {
		if emit != nil {
			emit(bootstrap.StepEvent{Index: i, Name: "step"})
		}
	}
	return nil, r.err
}

func TestBootstrapProgressPipe_FatalMapsToFatalMsg(t *testing.T) {
	cause := errors.New("permission denied")
	fatal := bootstrap.NewFatal("Portal failed to raise the restoring flag: permission denied", cause)
	fatal.Step = 2
	runner := &fatalAfterRunner{emitSteps: 1, err: fatal}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	fatalMsg := fatalMsgFrom(t, drainPipe(t, pipe.receiver()))

	if fatalMsg.FailedStep != 2 {
		t.Errorf("BootstrapFatalMsg.FailedStep = %d, want 2 (the step the fatal names)", fatalMsg.FailedStep)
	}
	if fatalMsg.Message != fatal.UserMessage {
		t.Errorf("BootstrapFatalMsg.Message = %q, want %q (FatalError.UserMessage)", fatalMsg.Message, fatal.UserMessage)
	}
	if !errors.Is(fatalMsg.Err, cause) {
		t.Errorf("BootstrapFatalMsg.Err did not carry the fatal cause; got %v", fatalMsg.Err)
	}
	if _, ok := errors.AsType[*bootstrap.FatalError](fatalMsg.Err); !ok {
		t.Error("BootstrapFatalMsg.Err is not a *bootstrap.FatalError (exit classification would miss it)")
	}
}

// bootstrap.lock's fatal comes before any step, so it fails none of them.
func TestBootstrapProgressPipe_FatalNamingNoStepFailsNone(t *testing.T) {
	fatal := bootstrap.NewFatal("Portal gave up waiting for another Portal to finish starting: boom", errors.New("boom"))
	runner := &fatalAfterRunner{emitSteps: 0, err: fatal}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	if got := fatalMsgFrom(t, drainPipe(t, pipe.receiver())).FailedStep; got != 0 {
		t.Errorf("BootstrapFatalMsg.FailedStep = %d, want 0 (no step failed)", got)
	}
}

func TestBootstrapProgressPipe_NonFatalStillCompletes(t *testing.T) {
	runner := &emittingRunner{steps: 7}
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

// fatalMsgFrom is the fatal msgs carry, failing the test on a completion.
func fatalMsgFrom(t *testing.T, msgs []tea.Msg) tui.BootstrapFatalMsg {
	t.Helper()
	var fatalMsg *tui.BootstrapFatalMsg
	for _, m := range msgs {
		if _, ok := m.(tui.BootstrapCompleteMsg); ok {
			t.Error("fatal run produced a BootstrapCompleteMsg; want a BootstrapFatalMsg")
		}
		if fm, ok := m.(tui.BootstrapFatalMsg); ok {
			fatalMsg = &fm
		}
	}
	if fatalMsg == nil {
		t.Fatal("fatal run never produced a tui.BootstrapFatalMsg")
	}
	return *fatalMsg
}
