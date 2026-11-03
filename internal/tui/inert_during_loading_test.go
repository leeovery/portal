package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

type mutationRecorder struct {
	killCalls     int
	renameCalls   int
	createCalls   int
	attachCalls   int
	enumCalls     int
	listSessCalls int
}

func (r *mutationRecorder) KillSession(string) error        { r.killCalls++; return nil }
func (r *mutationRecorder) RenameSession(_, _ string) error { r.renameCalls++; return nil }
func (r *mutationRecorder) CreateFromDir(string, []string) (string, error) {
	r.createCalls++
	return "", nil
}

func (r *mutationRecorder) Run(string, int, int) tea.Cmd {
	r.attachCalls++
	return nil
}

func (r *mutationRecorder) ListWindowsAndPanesInSession(string) ([]tmux.WindowGroup, error) {
	r.enumCalls++
	return nil, nil
}

func (r *mutationRecorder) ListSessions() ([]Session, error) {
	r.listSessCalls++
	return nil, errors.New("loading-page ListSessions must not be reached")
}

func (r *mutationRecorder) totalMutations() int {
	return r.killCalls + r.renameCalls + r.createCalls + r.attachCalls + r.enumCalls
}

func TestInertDuringLoading_NoMutationFromLiveEventLoop(t *testing.T) {
	rec := &mutationRecorder{}

	m := New(rec,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithKiller(rec),
		WithRenamer(rec),
		WithSessionCreator(rec),
		WithPreviewAttachPipeline(rec),
		WithEnumerator(rec),
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
			"(kill=%d rename=%d create=%d attach=%d enum=%d)",
			got, rec.killCalls, rec.renameCalls, rec.createCalls, rec.attachCalls, rec.enumCalls)
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
