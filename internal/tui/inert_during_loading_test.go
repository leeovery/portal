package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type mutationRecorder struct {
	killCalls     int
	labelCalls    int
	createCalls   int
	historyCalls  int
	listSessCalls int
}

func (r *mutationRecorder) KillSession(string) error   { r.killCalls++; return nil }
func (r *mutationRecorder) SetLabel(_, _ string) error { r.labelCalls++; return nil }
func (r *mutationRecorder) CreateFromDir(string, []string) (string, error) {
	r.createCalls++
	return "", nil
}

func (r *mutationRecorder) History(context.Context, string, int) ([]byte, error) {
	r.historyCalls++
	return nil, nil
}

func (r *mutationRecorder) ListSessions() ([]Session, error) {
	r.listSessCalls++
	return nil, errors.New("loading-page ListSessions must not be reached")
}

func (r *mutationRecorder) totalMutations() int {
	return r.killCalls + r.labelCalls + r.createCalls + r.historyCalls
}

func TestInertDuringLoading_NoMutationFromLiveEventLoop(t *testing.T) {
	rec := &mutationRecorder{}

	m := New(rec,
		WithBootstrapping(idleBootstrap),
		WithKiller(rec),
		WithLabeller(rec),
		WithSessionCreator(rec),
		WithHistory(rec),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading, got %v", model.(Model).ActivePage())
	}

	keys := []tea.KeyPressMsg{
		runeKeyMsg('k'),
		runeKeyMsg('x'),
		runeKeyMsg('r'),
		runeKeyMsg('n'),
		runeKeyMsg('s'),
		runeKeyMsg('j'),
		runeKeyMsg('g'),
		runeKeyMsg('G'),
		runeKeyMsg('/'),
		runeKeyMsg('y'),
		runeKeyMsg('a'),
		{Code: tea.KeyEnter},
		{Code: tea.KeySpace},
		{Code: tea.KeyTab},
		{Code: tea.KeyEscape},
		{Code: tea.KeyUp},
		{Code: tea.KeyDown},
	}
	for _, k := range keys {
		var cmd tea.Cmd
		model, cmd = model.Update(k)
		if model.(Model).ActivePage() != PageLoading {
			t.Fatalf("a key (%v) navigated off PageLoading during loading — the page must stay inert", k)
		}
		if cmd != nil {
			if msg := cmd(); msg != nil {
				model, _ = model.Update(msg)
			}
		}
	}

	for i := 1; i <= 10; i++ {
		model, _ = model.Update(BootstrapProgressMsg{Index: i})
		if model.(Model).ActivePage() != PageLoading {
			t.Fatalf("a progress event (index %d) transitioned off PageLoading — only the terminal complete event may", i)
		}
	}

	if got := rec.totalMutations(); got != 0 {
		t.Errorf("inert-during-loading VIOLATED: %d mutating seam call(s) reached during loading "+
			"(kill=%d rename=%d create=%d history=%d)",
			got, rec.killCalls, rec.labelCalls, rec.createCalls, rec.historyCalls)
	}
	if rec.listSessCalls != 0 {
		t.Errorf("inert-during-loading: the loading-page key arm issued %d ListSessions call(s); "+
			"the loading page must not enumerate sessions from key input (Init's frame-one "+
			"fetch is the only permitted read, and it ran before this storm)", rec.listSessCalls)
	}
}

func runeKeyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}
