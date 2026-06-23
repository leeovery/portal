package cmd

// Task spectrum-tui-design-5-2 — cmd-side progress channel + goroutine wrapper.
//
// These tests cover the bootstrapProgressPipe: the buffered channel that
// carries serverStarted + per-step events + the terminal done marker from the
// orchestrator goroutine to the loading-page TUI's receiver tea.Cmd. The pipe
// must: run the orchestrator in a goroutine, emit one event per real step in
// order, carry serverStarted on the terminal event, close the channel on return
// (success or fatal), and have its receiver stop re-issuing on a closed channel
// (no goroutine leak, no blocked receive after Quit).
//
// Tests mutate no shared package state but MUST NOT use t.Parallel (cmd rule).

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

// emittingRunner is a bootstrap.Runner that drives the context-carried progress
// emitter through N steps before returning the configured (started, warnings,
// err) tuple — mirroring the real orchestrator's emit-per-step contract without
// standing up the eleven step seams.
type emittingRunner struct {
	steps    int
	started  bool
	warnings []bootstrap.Warning
	err      error
}

func (r *emittingRunner) Run(ctx context.Context) (bool, []bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	if r.err == nil { // a fatal abort emits no step events past the failure
		for i := 1; i <= r.steps; i++ {
			if emit != nil {
				emit(bootstrap.StepEvent{Index: i, Name: "step"})
			}
		}
	}
	return r.started, r.warnings, r.err
}

// drainPipe collects every tea.Msg the receiver yields until the channel closes
// (the receiver returns a bootstrapChannelClosedMsg, which stops the loop). It
// re-invokes the receiver synchronously, mirroring the model's re-issue-on-
// progress behaviour but without the Bubble Tea runtime.
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
			// A BootstrapCompleteMsg (terminal) is followed by a channel close;
			// keep pulling so the loop observes the close sentinel and returns.
		case <-deadline:
			t.Fatal("drainPipe timed out — receiver blocked (channel never closed?)")
		}
	}
}

func TestBootstrapProgressPipe_EmitsPerStepThenTerminalThenCloses(t *testing.T) {
	runner := &emittingRunner{steps: 11, started: true}
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
	if progressCount != 11 {
		t.Errorf("progress msgs = %d, want 11 (one per real step)", progressCount)
	}
	if !sawComplete {
		t.Error("never received terminal BootstrapCompleteMsg")
	}
	if !sawClosed {
		t.Error("receiver never reported a closed channel")
	}
}

func TestBootstrapProgressPipe_PreservesStepOrder(t *testing.T) {
	runner := &emittingRunner{steps: 11, started: true}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	msgs := drainPipe(t, pipe.receiver())

	var order []int
	for _, m := range msgs {
		if pm, ok := m.(tui.BootstrapProgressMsg); ok {
			order = append(order, pm.Index)
		}
	}
	if len(order) != 11 {
		t.Fatalf("got %d progress indices, want 11: %v", len(order), order)
	}
	for i, idx := range order {
		if idx != i+1 {
			t.Errorf("progress order[%d] = %d, want %d (exact step order)", i, idx, i+1)
		}
	}
}

func TestBootstrapProgressPipe_CarriesServerStartedOnTerminalEvent(t *testing.T) {
	runner := &emittingRunner{steps: 1, started: true}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)

	_ = drainPipe(t, pipe.receiver())

	if !pipe.ServerStarted() {
		t.Error("pipe.ServerStarted() = false after a cold (started=true) run; serverStarted not carried over the channel")
	}
}

func TestBootstrapProgressPipe_FastColdBoot_ZeroRestoreItems(t *testing.T) {
	// M=0 restore: the orchestrator still emits its per-step events then the
	// terminal event. steps=11 with no per-session counter exercises this.
	runner := &emittingRunner{steps: 11, started: true}
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
	if progressCount != 11 || !sawComplete {
		t.Errorf("fast cold boot: progress=%d complete=%v, want 11 + terminal", progressCount, sawComplete)
	}
}

func TestBootstrapProgressPipe_ClosesChannelOnFatal(t *testing.T) {
	// task 5-6 owns the full fatal contract; here we only assert the pipe still
	// closes the channel (no leak) and surfaces the fatal so 5-6 can slot in.
	boom := errors.New("ensure-server boom")
	runner := &emittingRunner{steps: 0, started: true, err: boom}
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
	if !errors.Is(pipe.Err(), boom) {
		t.Errorf("pipe.Err() = %v, want %v carried through for task 5-6", pipe.Err(), boom)
	}
}

// TestBootstrapProgressPipe_CarriesWarningsOnTerminalEvent asserts the task-5-7
// carry-forward: soft warnings the orchestrator accumulated ride the terminal
// channel EVENT onto tui.BootstrapCompleteMsg.Warnings (value copies read off the
// event), not the pipe's struct fields — so the receiver never races the
// goroutine's writes. Multiple warnings preserve orchestrator-observation order.
func TestBootstrapProgressPipe_CarriesWarningsOnTerminalEvent(t *testing.T) {
	warnings := []bootstrap.Warning{
		{Lines: []string{"saver is down", "restart to recover"}},
		{Lines: []string{"sessions.json corrupt"}},
	}
	runner := &emittingRunner{steps: 1, started: true, warnings: warnings}
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

func TestBootstrapProgressPipe_ReceiverStopsReIssuingOnClose(t *testing.T) {
	runner := &emittingRunner{steps: 3, started: true}
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), runner)
	receiver := pipe.receiver()

	// Drain to completion (channel closes).
	_ = drainPipe(t, receiver)

	// A post-close receive must return the closed sentinel WITHOUT blocking —
	// the model's BootstrapProgressMsg arm re-issues, but the closed channel
	// means no blocked receive remains after Quit (no goroutine leak).
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
