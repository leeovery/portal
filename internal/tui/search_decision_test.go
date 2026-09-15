package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// decisionCounter is the closure under test plus the call count every gate
// assertion in this file is written against.
type decisionCounter struct {
	name  string
	err   error
	calls int
}

func (d *decisionCounter) decide() (string, error) {
	d.calls++
	return d.name, d.err
}

func searchDecisionModel(t *testing.T, d *decisionCounter) tea.Model {
	t.Helper()
	lister := postloadStubLister{sessions: []tmux.Session{{Name: "portal", Windows: 1}}}
	receiver := func() tea.Msg { return nil }
	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(receiver),
		WithSearchForm("port"),
		WithSearchDecision(d.decide),
	)
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

// transitionThroughDecision drives both loading gates and then delivers the
// message the dispatched decision command answers with, which is how the model
// reaches its post-decision state outside a running Bubble Tea program. Either
// gate may be the one that dispatches, so both returned commands are examined.
func transitionThroughDecision(t *testing.T, model tea.Model, warnings []BootstrapWarning) (tea.Model, tea.Cmd) {
	t.Helper()

	var decision tea.Msg
	for _, msg := range []tea.Msg{LoadingMinElapsedMsg{}, BootstrapCompleteMsg{Warnings: warnings}} {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		if cmd == nil {
			continue
		}
		if answer := cmd(); isDecisionMsg(answer) {
			decision = answer
		}
	}
	if decision == nil {
		t.Fatal("neither loading gate dispatched a decision command")
	}
	return model.Update(decision)
}

func TestSearchDecision_AttachesSingleMatchWithoutPainting(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model, cmd := transitionThroughDecision(t, searchDecisionModel(t, d), nil)

	m := model.(Model)
	if m.ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading — the picker must never be composed", m.ActivePage())
	}
	if m.Selected() != "portal-abc" {
		t.Errorf("Selected() = %q, want %q", m.Selected(), "portal-abc")
	}
	if !m.SearchAttached() {
		t.Error("SearchAttached() = false, want true")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected a quit command on the attach decision")
	}
}

func TestSearchDecision_LeavesBufferedWarningsUnsurfacedOnAttach(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, _ := transitionThroughDecision(t, searchDecisionModel(t, d), warnings)

	m := model.(Model)
	if len(m.BufferedWarnings()) != 1 {
		t.Fatalf("BufferedWarnings() = %#v, want the single buffered warning", m.BufferedWarnings())
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("a notice band owns the slot on an attach; the warnings must stay unsurfaced")
	}
}

func TestSearchDecision_OpensPickerWhenNoSession(t *testing.T) {
	d := &decisionCounter{}
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, _ := transitionThroughDecision(t, searchDecisionModel(t, d), warnings)

	m := model.(Model)
	if m.ActivePage() != PageSessions {
		t.Fatalf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want empty", m.Selected())
	}
	if m.SearchAttached() {
		t.Error("SearchAttached() = true on a picker decision, want false")
	}
	role, _, ok := m.activeNoticeBand()
	if !ok || role != bandWarning {
		t.Errorf("post-transition band role = %v ok = %v, want bandWarning true", role, ok)
	}
}

func TestSearchDecision_RecordsReadFailureWithoutFatal(t *testing.T) {
	readErr := errors.New("no server running")
	d := &decisionCounter{err: readErr}
	model, cmd := transitionThroughDecision(t, searchDecisionModel(t, d), nil)

	m := model.(Model)
	if !errors.Is(m.SearchError(), readErr) {
		t.Errorf("SearchError() = %v, want %v", m.SearchError(), readErr)
	}
	if m.FatalError() != nil {
		t.Errorf("FatalError() = %v, want nil — a read failure is not a bootstrap fatal", m.FatalError())
	}
	if m.ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading", m.ActivePage())
	}
	if !isQuitCmd(cmd) {
		t.Error("expected a quit command on the read-failure decision")
	}
}

func TestSearchDecision_HeldUntilMinimumSpanElapses(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	model, cmd := model.Update(BootstrapCompleteMsg{})
	if cmd != nil {
		t.Fatalf("complete alone dispatched %T, want nothing until the span elapses", cmd())
	}

	model, _ = transitionThroughDecision(t, model, nil)
	if d.calls != 1 {
		t.Fatalf("decision calls = %d after both gates, want 1", d.calls)
	}
	if model.(Model).Selected() != "portal-abc" {
		t.Errorf("Selected() = %q, want the decided session", model.(Model).Selected())
	}
}

func TestSearchDecision_HeldUntilBootstrapCompletes(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	model, cmd := model.Update(LoadingMinElapsedMsg{})
	if cmd != nil {
		t.Fatalf("min-elapsed alone dispatched %T, want nothing until the bootstrap completes", cmd())
	}

	model, _ = transitionThroughDecision(t, model, nil)
	if d.calls != 1 {
		t.Fatalf("decision calls = %d after both gates, want 1", d.calls)
	}
	if model.(Model).Selected() != "portal-abc" {
		t.Errorf("Selected() = %q, want the decided session", model.(Model).Selected())
	}
}

func TestSearchDecision_NotIssuedOnProgress(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	for _, msg := range []BootstrapProgressMsg{
		{Index: 1},
		{Index: 6, RestoreN: 2, RestoreM: 5},
		{Index: 10},
	} {
		model, _ = model.Update(msg)
	}

	if d.calls != 0 {
		t.Errorf("decision calls = %d on progress events, want 0", d.calls)
	}
	if model.(Model).ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading", model.(Model).ActivePage())
	}
}

func TestSearchDecision_NotIssuedAfterBootstrapFatal(t *testing.T) {
	fatalErr := errors.New("step 8 failed")
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	model, _ = model.Update(BootstrapFatalMsg{FailedStep: 8, Message: "could not clear marker", Err: fatalErr})
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{})

	m := model.(Model)
	if d.calls != 0 {
		t.Errorf("decision calls = %d after a fatal, want 0", d.calls)
	}
	if !errors.Is(m.FatalError(), fatalErr) {
		t.Errorf("FatalError() = %v, want the carried fatal", m.FatalError())
	}
	if m.ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading", m.ActivePage())
	}
}

func TestSearchDecision_IssuedExactlyOnce(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	dispatches := 0
	for _, msg := range []tea.Msg{LoadingMinElapsedMsg{}, BootstrapCompleteMsg{}, BootstrapCompleteMsg{}, LoadingMinElapsedMsg{}} {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		if cmd == nil {
			continue
		}
		if answer := cmd(); isDecisionMsg(answer) {
			dispatches++
			model, _ = model.Update(answer)
		}
	}

	if dispatches != 1 {
		t.Errorf("decision dispatches = %d across a duplicated gate sequence, want 1", dispatches)
	}
	if d.calls != 1 {
		t.Errorf("decision calls = %d across a duplicated gate sequence, want 1", d.calls)
	}
}

func TestSearchDecision_CtrlCBeforeDecisionSelectsNothing(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	model, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	m := model.(Model)
	if d.calls != 0 {
		t.Errorf("decision calls = %d on a Ctrl-C quit, want 0", d.calls)
	}
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want empty", m.Selected())
	}
	if !isQuitCmd(cmd) {
		t.Error("expected Ctrl-C on the loading page to quit")
	}
}

func TestSearchDecision_AbsentClosureLeavesTransitionUnchanged(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, _ := transitionWithWarnings(coldTUIModel(t, []tmux.Session{{Name: "portal", Windows: 1}}), warnings)

	m := model.(Model)
	if m.ActivePage() != PageSessions {
		t.Fatalf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want empty", m.Selected())
	}
	if m.SearchAttached() {
		t.Error("SearchAttached() = true with no decision closure, want false")
	}
	if m.SearchError() != nil {
		t.Errorf("SearchError() = %v, want nil", m.SearchError())
	}
	role, _, ok := m.activeNoticeBand()
	if !ok || role != bandWarning {
		t.Errorf("band role = %v ok = %v, want bandWarning true", role, ok)
	}
}

func TestSearchDecision_HoldsLoadingPageWhileInFlight(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model, cmd := transitionWithWarnings(searchDecisionModel(t, d), nil)

	m := model.(Model)
	if m.ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading while the decision is in flight", m.ActivePage())
	}
	if d.calls != 0 {
		t.Errorf("decision calls = %d before the command ran, want 0 — the closure must not run on the update goroutine", d.calls)
	}
	if cmd == nil {
		t.Fatal("the gate dispatched no decision command")
	}
	if msg := cmd(); !isDecisionMsg(msg) {
		t.Errorf("dispatched command answered with %T, want searchDecisionMsg", msg)
	}
	if d.calls != 1 {
		t.Errorf("decision calls = %d after the command ran, want 1", d.calls)
	}
}

func isDecisionMsg(msg tea.Msg) bool {
	_, ok := msg.(searchDecisionMsg)
	return ok
}

func TestSearchDecision_CtrlCWhileInFlightSelectsNothing(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model, _ := transitionWithWarnings(searchDecisionModel(t, d), nil)

	model, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	m := model.(Model)
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want empty", m.Selected())
	}
	if m.SearchAttached() {
		t.Error("SearchAttached() = true after a cancel, want false")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected Ctrl-C to quit while the decision is in flight")
	}
}

func TestSearchDecision_MessageAfterFatalLeavesErrorFrameStanding(t *testing.T) {
	fatalErr := errors.New("step 8 failed")
	d := &decisionCounter{name: "portal-abc"}
	model := searchDecisionModel(t, d)

	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, cmd := model.Update(BootstrapCompleteMsg{})
	if cmd == nil {
		t.Fatal("the gate dispatched no decision command")
	}
	answer := cmd()

	model, _ = model.Update(BootstrapFatalMsg{FailedStep: 8, Message: "could not clear marker", Err: fatalErr})
	model, quit := model.Update(answer)

	m := model.(Model)
	if !errors.Is(m.FatalError(), fatalErr) {
		t.Errorf("FatalError() = %v, want the carried fatal", m.FatalError())
	}
	if m.ActivePage() != PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading — the error frame must stand", m.ActivePage())
	}
	if m.Selected() != "" || m.SearchAttached() {
		t.Errorf("Selected() = %q SearchAttached() = %v, want empty and false", m.Selected(), m.SearchAttached())
	}
	if quit != nil {
		t.Errorf("a decision arriving after a fatal returned %T, want nothing", quit())
	}
}
