package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

type emittingRunner struct {
	steps    int
	warnings []bootstrap.Warning
	err      error
}

func (r *emittingRunner) Run(ctx context.Context) ([]bootstrap.Warning, error) {
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
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
	if !errors.Is(pipe.Err(), boom) {
		t.Errorf("pipe.Err() = %v, want %v carried through", pipe.Err(), boom)
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
