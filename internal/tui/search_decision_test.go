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

func TestSearchDecision_AttachesSingleMatchWithoutPainting(t *testing.T) {
	d := &decisionCounter{name: "portal-abc"}
	model, cmd := transitionWithWarnings(searchDecisionModel(t, d), nil)

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
	model, _ := transitionWithWarnings(searchDecisionModel(t, d), warnings)

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
	model, _ := transitionWithWarnings(searchDecisionModel(t, d), warnings)

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
	model, cmd := transitionWithWarnings(searchDecisionModel(t, d), nil)

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

	model, _ = model.Update(BootstrapCompleteMsg{})
	if d.calls != 0 {
		t.Fatalf("decision calls = %d after complete alone, want 0", d.calls)
	}

	model, _ = model.Update(LoadingMinElapsedMsg{})
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

	model, _ = model.Update(LoadingMinElapsedMsg{})
	if d.calls != 0 {
		t.Fatalf("decision calls = %d after min-elapsed alone, want 0", d.calls)
	}

	model, _ = model.Update(BootstrapCompleteMsg{})
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

	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{})
	_, _ = model.Update(LoadingMinElapsedMsg{})

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
