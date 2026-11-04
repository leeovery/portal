package tui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tui"
)

func TestBootstrapProgressMsg_ReIssuesReceiver(t *testing.T) {
	lister := &mockSessionLister{sessions: []tui.Session{}}
	reissued := make(chan struct{}, 1)
	receiver := tea.Cmd(func() tea.Msg {
		select {
		case reissued <- struct{}{}:
		default:
		}
		return tui.BootstrapProgressMsg{Index: 2}
	})
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, cmd := model.Update(tui.BootstrapProgressMsg{Index: 1})
	updated := model.(tui.Model)

	if updated.ActivePage() != tui.PageLoading {
		t.Errorf("progress event drove off PageLoading; got %d", updated.ActivePage())
	}
	if cmd == nil {
		t.Fatal("BootstrapProgressMsg returned nil cmd; want the re-issued receiver")
	}
	if _, ok := cmd().(tui.BootstrapProgressMsg); !ok {
		t.Error("re-issued cmd did not produce a BootstrapProgressMsg")
	}
	select {
	case <-reissued:
	case <-time.After(time.Second):
		t.Error("receiver was not re-issued by the BootstrapProgressMsg arm")
	}
}

func TestBootstrapComplete_TransitionGatedOnTerminalEvent(t *testing.T) {
	lister := &mockSessionLister{sessions: []tui.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(tui.LoadingMinElapsedMsg{})
	for i := 1; i <= 10; i++ {
		model, _ = model.Update(tui.BootstrapProgressMsg{Index: i})
	}
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Fatalf("transitioned before terminal event; got %d", model.(tui.Model).ActivePage())
	}

	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if model.(tui.Model).ActivePage() == tui.PageLoading {
		t.Error("did not transition to Sessions on terminal BootstrapCompleteMsg")
	}
}

func TestConcurrentInit_DoesNotSynthesizeBootstrapComplete(t *testing.T) {
	lister := &mockSessionLister{sessions: []tui.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
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
		}
	}
}

func TestConcurrentInit_IncludesProgressReceiver(t *testing.T) {
	lister := &mockSessionLister{sessions: []tui.Session{}}
	hit := make(chan struct{}, 1)
	receiver := tea.Cmd(func() tea.Msg {
		hit <- struct{}{}
		return tui.BootstrapProgressMsg{Index: 1}
	})
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
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

func TestLoadingInert_NoEnumerationBeforeComplete(t *testing.T) {
	lister := &mockSessionLister{sessions: []tui.Session{{Name: "a"}}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(tui.SessionsMsg{Sessions: []tui.Session{{Name: "a"}}})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Error("model left PageLoading before terminal complete (not inert)")
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Error("page-nav key was honoured during loading; want inert")
	}
}
