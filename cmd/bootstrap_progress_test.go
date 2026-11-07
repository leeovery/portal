package cmd

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

type emittingRunner struct {
	// waited sends the event a long wait for another bootstrap sends, before
	// any step.
	waited   bool
	steps    int
	warnings []bootstrap.Warning
	err      error
}

func (r *emittingRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	if r.waited && emit != nil {
		emit(bootstrap.StepEvent{Waiting: true})
	}
	if r.err == nil {
		for i := 1; i <= r.steps; i++ {
			if emit != nil {
				emit(bootstrap.StepEvent{Index: i, Name: "step"})
			}
		}
	}
	return r.warnings, r.err
}

func drainPipe(t *testing.T, receiver tea.Cmd) []tea.Msg {
	t.Helper()
	var msgs []tea.Msg
	deadline := time.After(2 * time.Second)
	for {
		got := make(chan tea.Msg, 1)
		go func() { got <- receiver() }()
		select {
		case msg := <-got:
			msgs = append(msgs, msg)
			if _, closed := msg.(bootstrapChannelClosedMsg); closed {
				return msgs
			}
		case <-deadline:
			t.Fatal("drainPipe timed out — receiver blocked (channel never closed?)")
		}
	}
}

func TestBootstrapProgressPipe_EmitsPerStepThenTerminalThenCloses(t *testing.T) {
	runner := &emittingRunner{steps: 7}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var progressCount int
	var sawComplete, sawClosed bool
	for _, m := range msgs {
		switch m.(type) {
		case tui.BootstrapProgressMsg:
			progressCount++
		case tui.BootstrapCompleteMsg:
			sawComplete = true
		case bootstrapChannelClosedMsg:
			sawClosed = true
		}
	}
	if progressCount != 7 {
		t.Errorf("progress msgs = %d, want 7 (one per real step)", progressCount)
	}
	if !sawComplete {
		t.Error("never received terminal BootstrapCompleteMsg")
	}
	if !sawClosed {
		t.Error("receiver never reported a closed channel")
	}
}

func TestBootstrapProgressPipe_PreservesStepOrder(t *testing.T) {
	runner := &emittingRunner{steps: 7}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var order []int
	for _, m := range msgs {
		if pm, ok := m.(tui.BootstrapProgressMsg); ok {
			order = append(order, pm.Index)
		}
	}
	if len(order) != 7 {
		t.Fatalf("got %d progress indices, want 7: %v", len(order), order)
	}
	for i, idx := range order {
		if idx != i+1 {
			t.Errorf("progress order[%d] = %d, want %d (exact step order)", i, idx, i+1)
		}
	}
}

func TestBootstrapProgressPipe_CarriesTheWaitAheadOfTheSteps(t *testing.T) {
	runner := &emittingRunner{waited: true, steps: 1}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	want := []tui.BootstrapProgressMsg{{Waiting: true}, {Index: 1}}
	var got []tui.BootstrapProgressMsg
	for _, m := range msgs {
		if pm, ok := m.(tui.BootstrapProgressMsg); ok {
			got = append(got, pm)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("progress msgs = %+v, want %+v", got, want)
	}
}

func TestBootstrapProgressPipe_FastColdBoot_ZeroRestoreItems(t *testing.T) {
	runner := &emittingRunner{steps: 7}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var progressCount int
	var sawComplete bool
	for _, m := range msgs {
		switch m.(type) {
		case tui.BootstrapProgressMsg:
			progressCount++
		case tui.BootstrapCompleteMsg:
			sawComplete = true
		}
	}
	if progressCount != 7 || !sawComplete {
		t.Errorf("fast cold boot: progress=%d complete=%v, want 7 + terminal", progressCount, sawComplete)
	}
}

func TestBootstrapProgressPipe_ClosesChannelOnFatal(t *testing.T) {
	boom := errors.New("ensure-server boom")
	runner := &emittingRunner{steps: 0, err: boom}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var sawClosed bool
	for _, m := range msgs {
		if _, ok := m.(bootstrapChannelClosedMsg); ok {
			sawClosed = true
		}
	}
	if !sawClosed {
		t.Error("channel was not closed on fatal — goroutine would leak")
	}
	if fatal := fatalMsgFrom(t, msgs); !errors.Is(fatal.Err, boom) {
		t.Errorf("fatal Err = %v, want %v carried through", fatal.Err, boom)
	}
}

func TestBootstrapProgressPipe_CarriesWarningsOnTerminalEvent(t *testing.T) {
	warnings := []bootstrap.Warning{
		{Lines: []string{"saver is down", "restart to recover"}},
		{Lines: []string{"sessions.json corrupt"}},
	}
	runner := &emittingRunner{steps: 1, warnings: warnings}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var complete *tui.BootstrapCompleteMsg
	for _, m := range msgs {
		if bc, ok := m.(tui.BootstrapCompleteMsg); ok {
			complete = &bc
		}
	}
	if complete == nil {
		t.Fatal("never received terminal BootstrapCompleteMsg")
	}
	if len(complete.Warnings) != 2 {
		t.Fatalf("BootstrapCompleteMsg.Warnings len = %d, want 2", len(complete.Warnings))
	}
	if len(complete.Warnings[0].Lines) != 2 || complete.Warnings[0].Lines[0] != "saver is down" {
		t.Errorf("first warning off the event = %#v", complete.Warnings[0])
	}
	if len(complete.Warnings[1].Lines) != 1 || complete.Warnings[1].Lines[0] != "sessions.json corrupt" {
		t.Errorf("second warning off the event = %#v", complete.Warnings[1])
	}
}

// unwindingRunner is a bootstrap that runs until its context ends, then takes
// a moment to unwind, as a restore cut short lowers the restoring flag on its
// way out. One that ignores its context runs until released.
type unwindingRunner struct {
	ignoresItsEnd bool
	release       chan struct{}
	returned      atomic.Bool
}

func (r *unwindingRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	if r.ignoresItsEnd {
		<-r.release
	} else {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
	}
	r.returned.Store(true)
	return nil, ctx.Err()
}

func TestBootstrapProgressPipe_Unwind(t *testing.T) {
	t.Run("it ends the bootstrap's context and waits for it to return", func(t *testing.T) {
		runner := &unwindingRunner{}
		pipe := newBootstrapProgressPipe()
		pipe.start(context.Background(), runner)

		pipe.unwind()

		if !runner.returned.Load() {
			t.Error("unwind returned before the bootstrap did")
		}
	})

	t.Run("it gives up on a bootstrap that doesn't return past its bound", func(t *testing.T) {
		runner := &unwindingRunner{ignoresItsEnd: true, release: make(chan struct{})}
		t.Cleanup(func() { close(runner.release) })
		pipe := newBootstrapProgressPipe()
		pipe.unwindWait = 50 * time.Millisecond
		pipe.start(context.Background(), runner)
		start := time.Now()

		pipe.unwind()

		if took := time.Since(start); took < pipe.unwindWait || took > time.Second {
			t.Errorf("unwind took %s, want it giving up once %s had passed", took, pipe.unwindWait)
		}
		if runner.returned.Load() {
			t.Error("the bootstrap returned, want it still running")
		}
	})

	t.Run("after a bootstrap that finished it returns at once", func(t *testing.T) {
		pipe := newBootstrapProgressPipe()
		pipe.unwindWait = time.Minute
		pipe.start(context.Background(), &emittingRunner{steps: 6})
		_ = drainPipe(t, pipe.receiver())
		start := time.Now()

		pipe.unwind()

		if took := time.Since(start); took > time.Second {
			t.Errorf("unwind took %s over a bootstrap that had finished", took)
		}
	})
}

func TestBootstrapProgressPipe_ReceiverStopsReIssuingOnClose(t *testing.T) {
	runner := &emittingRunner{steps: 3}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)
	receiver := pipe.receiver()

	_ = drainPipe(t, receiver)

	got := make(chan tea.Msg, 1)
	go func() { got <- receiver() }()
	select {
	case msg := <-got:
		if _, ok := msg.(bootstrapChannelClosedMsg); !ok {
			t.Errorf("post-close receive = %T, want bootstrapChannelClosedMsg", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("post-close receive blocked — receiver did not detect the closed channel")
	}
}
