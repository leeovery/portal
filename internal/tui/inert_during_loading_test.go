package tui

// spectrum-tui-design-5-8 — Part C: inert-during-loading assertion.
//
// §10.2's race-containment property: while the orchestrator runs in a goroutine
// on the cold/TUI route, the live Bubble Tea event loop must perform NO tmux or
// state MUTATION until the terminal complete event dismisses the loading page.
// The loading page is "animation only" (5-2): no session enumeration that
// mutates, no page navigation, no attach/kill/rename/create until
// transitionFromLoading. This is the property that contains the race surface —
// the orchestrator owns tmux mutation during loading; the TUI touches nothing.
//
// This test proves it BEHAVIOURALLY: every mutating seam (Killer, Renamer,
// Creator, PreviewAttacher, the preview TmuxEnumerator) is a recording mock that
// panics-on-call-count, and a representative storm of key presses + progress
// messages is pumped through Update while the model is parked on PageLoading. If
// any keystroke or progress event reached a mutation path the recorder's count
// would be non-zero. ListSessions reads are permitted (a pure enumeration is not
// a mutation) but we record them too so the test documents that the loop issues
// zero session reads from the loading-page key arm.
//
// No t.Parallel: consistent with the tui test-surface convention.

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// mutationRecorder counts every call to a mutating tmux/state seam. A non-zero
// count after a loading-page key/progress storm means the inert property was
// violated — the live event loop drove a mutation while the orchestrator was
// still running.
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

func (r *mutationRecorder) ListSessions() ([]tmux.Session, error) {
	r.listSessCalls++
	return nil, errors.New("loading-page ListSessions must not be reached")
}

func (r *mutationRecorder) totalMutations() int {
	return r.killCalls + r.renameCalls + r.createCalls + r.attachCalls + r.enumCalls
}

// TestInertDuringLoading_NoMutationFromLiveEventLoop is the Part-C behavioural
// proof. On the cold/TUI route (loading page + non-nil progressReceiver) a storm
// of key presses and progress events drives the model while parked on
// PageLoading; the recorder must observe ZERO mutating seam calls and ZERO
// session enumerations from the loading-page key arm.
func TestInertDuringLoading_NoMutationFromLiveEventLoop(t *testing.T) {
	rec := &mutationRecorder{}

	m := New(rec,
		WithServerStarted(true), // → PageLoading
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithKiller(rec),
		WithRenamer(rec),
		WithSessionCreator(rec),
		WithPreviewAttachPipeline(rec),
		WithEnumerator(rec),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Sanity: we are on the loading page before the storm.
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading, got %v", model.(Model).ActivePage())
	}

	// Representative key storm: every key that on the SESSIONS page would drive a
	// mutation or navigation — kill (k/x-confirm), rename (r), new (n), attach
	// (enter), preview (space), project nav (x), filter (/), grouping toggle (s),
	// cursor moves (j/k/g/G), Tab, and a bare rune. On PageLoading every one of
	// these must be inert (the key arm returns m, nil).
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
		runeKeyMsg('y'), // a kill-confirm "yes" on the sessions modal path
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
		// Drain any returned cmd through one round-trip so a deferred mutation
		// command (if one were erroneously dispatched) would actually execute.
		if cmd != nil {
			if msg := cmd(); msg != nil {
				model, _ = model.Update(msg)
			}
		}
	}

	// Interleave progress events: these drive the loading-screen render and
	// re-issue the receiver, but must NEVER reach a mutation path.
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

// runeKeyMsg builds a single-rune tea.KeyPressMsg. Local helper so the storm
// above reads cleanly without repeating the Code/Text construction.
func runeKeyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}
