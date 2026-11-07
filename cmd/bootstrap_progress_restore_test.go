package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

type restoreEmittingRunner struct {
	m int
}

func (r *restoreEmittingRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	if emit != nil {
		for n := 1; n <= r.m; n++ {
			emit(bootstrap.StepEvent{Index: 5, Name: "Restore", RestoreN: n, RestoreM: r.m})
		}
	}
	return nil, nil
}

func TestBootstrapProgressPipe_ForwardsRestoreNMOntoProgressMsg(t *testing.T) {
	runner := &restoreEmittingRunner{m: 3}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var got [][2]int
	for _, m := range msgs {
		if pm, ok := m.(tui.BootstrapProgressMsg); ok {
			got = append(got, [2]int{pm.RestoreN, pm.RestoreM})
		}
	}
	want := [][2]int{{1, 3}, {2, 3}, {3, 3}}
	if len(got) != len(want) {
		t.Fatalf("restore progress msgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("restore progress[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// Emits more events than the channel buffer holds while nothing drains, and
// signals when its goroutine returns.
type blockingRunner struct {
	events int
	done   chan struct{}
}

func (r *blockingRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	for n := 1; n <= r.events; n++ {
		if emit != nil {
			emit(bootstrap.StepEvent{Index: 5, Name: "Restore", RestoreN: n, RestoreM: r.events})
		}
	}
	close(r.done)
	return nil, nil
}

func TestBootstrapProgressPipe_SendUnblocksOnContextCancel(t *testing.T) {
	// Without a ctx-guarded send the goroutine would block forever on the
	// (buffer+1)-th send; the overflow margin guarantees it gets there.
	ctx, cancel := context.WithCancel(context.Background())
	runner := &blockingRunner{events: bootstrapProgressBufferSize + 8, done: make(chan struct{})}
	pipe := newBootstrapProgressPipe()
	pipe.start(ctx, runner)

	time.Sleep(50 * time.Millisecond)
	select {
	case <-runner.done:
		t.Fatal("runner returned before cancel — buffer should have blocked it (test no longer exercises the guard)")
	default:
	}

	cancel()

	select {
	case <-runner.done:
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator goroutine never unblocked after ctx cancel — the naked send wedged forever")
	}
}
