package tui_test

// Task spectrum-tui-design-5-2 — progress-channel receiver + Update arm (cold/TUI path).
//
// On the §10.2 concurrent cold-boot route the model receives live per-step
// progress over a channel: a receiver tea.Cmd blocks on a channel receive and
// re-issues itself on every BootstrapProgressMsg, with the terminal
// BootstrapCompleteMsg driving the transition to Sessions. These tests assert
// the Update arm, the gated transition, and that the model stays inert during
// loading (no enumeration / no page nav before complete).

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// TestBootstrapProgressMsg_ReIssuesReceiver asserts the BootstrapProgressMsg
// Update arm returns a command (the re-issued receiver) so the next channel
// event is pulled, and stays on PageLoading (a progress event is not terminal).
func TestBootstrapProgressMsg_ReIssuesReceiver(t *testing.T) {
	lister := &mockSessionLister{sessions: []tmux.Session{}}
	// Wire a receiver that, when re-issued, yields a sentinel we can detect.
	reissued := make(chan struct{}, 1)
	receiver := tea.Cmd(func() tea.Msg {
		select {
		case reissued <- struct{}{}:
		default:
		}
		return tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"}
	})
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, cmd := model.Update(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	updated := model.(tui.Model)

	if updated.ActivePage() != tui.PageLoading {
		t.Errorf("progress event drove off PageLoading; got %d", updated.ActivePage())
	}
	if cmd == nil {
		t.Fatal("BootstrapProgressMsg returned nil cmd; want the re-issued receiver")
	}
	// Invoking the returned cmd must re-run the receiver (pull the next event).
	if _, ok := cmd().(tui.BootstrapProgressMsg); !ok {
		t.Error("re-issued cmd did not produce a BootstrapProgressMsg")
	}
	select {
	case <-reissued:
	case <-time.After(time.Second):
		t.Error("receiver was not re-issued by the BootstrapProgressMsg arm")
	}
}

// TestBootstrapComplete_TransitionGatedOnTerminalEvent asserts the model only
// transitions to Sessions once the terminal BootstrapCompleteMsg arrives (paired
// with LoadingMinElapsedMsg) — progress events alone never transition.
func TestBootstrapComplete_TransitionGatedOnTerminalEvent(t *testing.T) {
	lister := &mockSessionLister{sessions: []tmux.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"} })
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Min elapsed, plus several progress events — still on loading.
	model, _ = model.Update(tui.LoadingMinElapsedMsg{})
	for i := 1; i <= 11; i++ {
		model, _ = model.Update(tui.BootstrapProgressMsg{Index: i, Name: "step"})
	}
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Fatalf("transitioned before terminal event; got %d", model.(tui.Model).ActivePage())
	}

	// Terminal event arrives — now transition.
	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if model.(tui.Model).ActivePage() == tui.PageLoading {
		t.Error("did not transition to Sessions on terminal BootstrapCompleteMsg")
	}
}

// TestConcurrentInit_DoesNotSynthesizeBootstrapComplete asserts that on the
// concurrent route (a progress receiver is wired) Init does NOT synthesize
// BootstrapCompleteMsg — the terminal event must come over the channel instead.
func TestConcurrentInit_DoesNotSynthesizeBootstrapComplete(t *testing.T) {
	lister := &mockSessionLister{sessions: []tmux.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"} })
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func(cmd tea.Cmd) { done <- cmd() }(c)
		select {
		case got := <-done:
			if _, ok := got.(tui.BootstrapCompleteMsg); ok {
				t.Error("concurrent Init synthesized BootstrapCompleteMsg; the channel must own the terminal event")
			}
		case <-time.After(50 * time.Millisecond):
			// loadingPadTick or a blocking receiver — ignore.
		}
	}
}

// TestConcurrentInit_IncludesProgressReceiver asserts the concurrent Init wires
// the progress receiver into its batch so channel events flow into Update.
func TestConcurrentInit_IncludesProgressReceiver(t *testing.T) {
	lister := &mockSessionLister{sessions: []tmux.Session{}}
	hit := make(chan struct{}, 1)
	receiver := tea.Cmd(func() tea.Msg {
		hit <- struct{}{}
		return tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"}
	})
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", cmd())
	}
	found := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func(cmd tea.Cmd) { done <- cmd() }(c)
		select {
		case got := <-done:
			if pm, ok := got.(tui.BootstrapProgressMsg); ok && pm.Index == 1 {
				found = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !found {
		t.Error("concurrent Init did not include the progress receiver in its batch")
	}
	select {
	case <-hit:
	case <-time.After(time.Second):
		t.Error("progress receiver was never invoked from Init's batch")
	}
}

// TestLoadingInert_NoEnumerationBeforeComplete asserts the model does not flip
// sessionsLoaded (no enumeration drives the page) while on PageLoading — even
// after a SessionsMsg arrives — until the terminal complete event transitions.
func TestLoadingInert_NoEnumerationBeforeComplete(t *testing.T) {
	lister := &mockSessionLister{sessions: []tmux.Session{{Name: "a"}}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"} })
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// A session list arrives during loading; it must be ingested but NOT drive
	// a transition (sessionsLoaded stays false; page stays PageLoading).
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{{Name: "a"}}})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Error("model left PageLoading before terminal complete (not inert)")
	}

	// A page-nav key during loading is swallowed (inert) — must not change page.
	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Error("page-nav key was honoured during loading; want inert")
	}
}
