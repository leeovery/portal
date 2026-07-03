package cmd

// Task spectrum-tui-design-5-3 — restore per-session N/M forwarding + the
// carry-forward ctx-guarded send.
//
// These tests cover two things on top of the 5-2 pipe:
//   - a restore per-session StepEvent (Index 6, RestoreN/RestoreM populated)
//     maps onto a tui.BootstrapProgressMsg carrying the same N/M (so task 5-4
//     can render "Restoring sessions (N/M)").
//   - the progress send unblocks on ctx cancellation rather than blocking the
//     orchestrator goroutine forever (the >63-events-after-Quit scenario).
//
// cmd rule: no t.Parallel.

import (
	"context"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

// restoreEmittingRunner drives the context-carried emitter through a fixed
// set of restore per-session events (Index 6, RestoreN/RestoreM populated)
// then returns the configured terminal tuple — mirroring how the real step 6
// streams N/M without standing up the ten step seams.
type restoreEmittingRunner struct {
	m       int
	started bool
}

func (r *restoreEmittingRunner) Run(ctx context.Context) (bool, []bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	if emit != nil {
		for n := 1; n <= r.m; n++ {
			emit(bootstrap.StepEvent{Index: 6, Name: "Restore", RestoreN: n, RestoreM: r.m})
		}
	}
	return r.started, nil, nil
}

func TestBootstrapProgressPipe_ForwardsRestoreNMOntoProgressMsg(t *testing.T) {
	runner := &restoreEmittingRunner{m: 3, started: true}
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

// blockingRunner emits more events than the channel buffer can hold while the
// receiver never drains — exercising the carry-forward ctx-guarded send. It
// signals when its goroutine returns so the test can assert the orchestrator
// goroutine unblocked on cancellation rather than wedging forever.
type blockingRunner struct {
	events int
	done   chan struct{}
}

func (r *blockingRunner) Run(ctx context.Context) (bool, []bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	for n := 1; n <= r.events; n++ {
		if emit != nil {
			emit(bootstrap.StepEvent{Index: 6, Name: "Restore", RestoreN: n, RestoreM: r.events})
		}
	}
	close(r.done)
	return true, nil, nil
}

func TestBootstrapProgressPipe_SendUnblocksOnContextCancel(t *testing.T) {
	// > buffer events with NO receiver draining: without a ctx-guarded send the
	// orchestrator goroutine blocks forever on the (buffer+1)-th send. With the
	// guard, cancelling ctx unblocks every pending send and the goroutine
	// returns. bootstrapProgressBufferSize+8 guarantees we overflow the buffer.
	ctx, cancel := context.WithCancel(context.Background())
	runner := &blockingRunner{events: bootstrapProgressBufferSize + 8, done: make(chan struct{})}
	pipe := newBootstrapProgressPipe()
	pipe.start(ctx, runner)

	// Let the goroutine fill the buffer and wedge on the blocking send.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-runner.done:
		t.Fatal("runner returned before cancel — buffer should have blocked it (test no longer exercises the guard)")
	default:
	}

	cancel()

	select {
	case <-runner.done:
		// good — the guarded send observed ctx.Done() and the runner returned.
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator goroutine never unblocked after ctx cancel — the naked send wedged forever")
	}
}
