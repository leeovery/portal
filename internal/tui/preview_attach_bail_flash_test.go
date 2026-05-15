package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// Tests for Phase 2 task 2-5: previewAttachBailMsg handler must emit an
// inline flash with the spec-exact wording, schedule a tick capturing the
// post-bump flashGen, and batch the flash tick alongside the existing
// sessions-list refresh from a single Update return.
//
// Spec § Session-killed-externally bail path > Behaviour pins the wording
// as `session "<name>" no longer exists` (literal double quotes, no
// trailing punctuation, no paraphrase).

// drainBatchCmds invokes cmd and, if it produces a tea.BatchMsg, returns
// the slice of contained Cmds. If cmd is nil or produces a non-batch msg,
// returns nil for callers to detect and react.
func drainBatchCmds(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return nil
	}
	return []tea.Cmd(batch)
}

// findFlashTickMsg invokes each cmd and returns the first flashTickMsg it
// finds, plus true. Returns zero/false when no cmd produces one. Skips
// nil cmds defensively.
func findFlashTickMsg(cmds []tea.Cmd) (flashTickMsg, bool) {
	for _, c := range cmds {
		if c == nil {
			continue
		}
		if ftm, ok := c().(flashTickMsg); ok {
			return ftm, true
		}
	}
	return flashTickMsg{}, false
}

// findRefreshedMsg invokes each cmd and returns the first
// previewSessionsRefreshedMsg it finds, plus true. Skips nil cmds.
func findRefreshedMsg(cmds []tea.Cmd) (previewSessionsRefreshedMsg, bool) {
	for _, c := range cmds {
		if c == nil {
			continue
		}
		if r, ok := c().(previewSessionsRefreshedMsg); ok {
			return r, true
		}
	}
	return previewSessionsRefreshedMsg{}, false
}

func TestPreviewAttachBail_SetsFlashWithExactSpecWording(t *testing.T) {
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "foo")

	want := `session "foo" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_FlashUsesLiteralDoubleQuotesAroundName(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	// Guard against accidental %q which would escape special characters or
	// use Go-style quoting. The flash must contain the raw double-quote
	// bytes wrapping the name.
	if !strings.Contains(got.flashText, `"alpha"`) {
		t.Errorf("flash must contain literal-double-quoted name `\"alpha\"`, got %q", got.flashText)
	}
}

func TestPreviewAttachBail_FlashHasNoTrailingPunctuation(t *testing.T) {
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "foo")

	last := got.flashText[len(got.flashText)-1]
	if last == '.' || last == '!' || last == '?' || last == ',' || last == ';' || last == ':' {
		t.Errorf("flashText must not end with punctuation, got %q (last byte %q)", got.flashText, string(last))
	}
	// Belt-and-braces: full-string equality is the canonical spec check.
	if got.flashText != `session "foo" no longer exists` {
		t.Errorf("flashText must equal exact spec wording, got %q", got.flashText)
	}
}

func TestPreviewAttachBail_BumpsFlashGen(t *testing.T) {
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	if m.flashGen != 0 {
		t.Fatalf("setup invariant: want flashGen=0, got %d", m.flashGen)
	}

	got, _ := pressSpaceThenBail(t, m, "foo")

	if got.flashGen != 1 {
		t.Errorf("flashGen after bail: want 1, got %d", got.flashGen)
	}
}

func TestPreviewAttachBail_ReturnsBatchWithRefreshAndTick(t *testing.T) {
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	_, cmd := pressSpaceThenBail(t, m, "foo")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from bail handler")
	}

	cmds := drainBatchCmds(cmd)
	if cmds == nil {
		t.Fatalf("expected tea.BatchMsg from bail cmd, got non-batch msg")
	}

	if _, ok := findRefreshedMsg(cmds); !ok {
		t.Errorf("expected refresh cmd in bail batch (producing previewSessionsRefreshedMsg)")
	}
	if _, ok := findFlashTickMsg(cmds); !ok {
		t.Errorf("expected flash tick cmd in bail batch (producing flashTickMsg)")
	}
}

func TestPreviewAttachBail_TickCapturesPostBumpFlashGen(t *testing.T) {
	// The tick must capture m.flashGen AFTER the setFlash bump so that on
	// fire it matches the live gen (which would still be 1 if no further
	// flash supersedes it). Capturing pre-bump (e.g. 0) would let any tick
	// scheduled with the stale gen mismatch and silently drop, defeating
	// the auto-clear behaviour.
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "foo")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from bail handler")
	}

	cmds := drainBatchCmds(cmd)
	ftm, ok := findFlashTickMsg(cmds)
	if !ok {
		t.Fatalf("expected flashTickMsg in bail batch")
	}
	if ftm.Gen != got.flashGen {
		t.Errorf("tick captured gen %d but live flashGen is %d (tick must capture post-bump value)", ftm.Gen, got.flashGen)
	}
	if ftm.Gen != 1 {
		t.Errorf("tick captured gen: want 1 (post-bump), got %d", ftm.Gen)
	}
}

func TestPreviewAttachBail_FlashVisibleBeforeRefreshResolves(t *testing.T) {
	// Spec § Render-frame ordering: visible response first. The flash must
	// be present in the rendered View immediately after the bail Update —
	// without dispatching/resolving the refresh batch. The killed-session
	// row may still appear in this stale frame; the flash text is what
	// matters for the immediate response.
	sessions := []tmux.Session{
		{Name: "foo", Windows: 1, Attached: false},
		{Name: "bar", Windows: 1, Attached: false},
	}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	postKill := []tmux.Session{{Name: "bar", Windows: 1, Attached: false}}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)
	m.termWidth = 80
	m.termHeight = 24

	got, _ := pressSpaceThenBail(t, m, "foo")

	rendered := got.View()
	want := `session "foo" no longer exists`
	if !strings.Contains(rendered, want) {
		t.Errorf("rendered View must contain flash %q before refresh resolves, got:\n%s", want, rendered)
	}
}

func TestPreviewAttachBail_SpecialCharsInNamePreservedVerbatim(t *testing.T) {
	// Names with non-trivial characters (spaces, dashes, dots, unicode)
	// must be embedded verbatim — no escaping, no quoting beyond the
	// literal surrounding double quotes.
	const weird = "my session-1.2 名字"
	sessions := []tmux.Session{{Name: weird, Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, weird)

	want := `session "` + weird + `" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText with special chars: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_EmptySessionNameDoesNotPanic(t *testing.T) {
	// Defensive: empty msg.Session must not panic. The flash will read
	// `session "" no longer exists` — odd, but well-formed and safe.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("bail with empty session panicked: %v", r)
		}
	}()

	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "")
	if got.activePage != PageSessions {
		t.Errorf("expected PageSessions after bail with empty name, got %v", got.activePage)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd even with empty session")
	}
	// And the flash text is well-formed (empty name slotted in).
	want := `session "" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText with empty name: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_BailHandlerNotUsingTeaSequence(t *testing.T) {
	// Discriminator: tea.Batch emits a tea.BatchMsg containing a slice of
	// Cmds. tea.Sequence emits a tea.sequenceMsg (unexported) shaped
	// differently. By asserting the returned cmd produces a tea.BatchMsg
	// we lock the implementation to tea.Batch composition.
	sessions := []tmux.Session{{Name: "foo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	_, cmd := pressSpaceThenBail(t, m, "foo")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from bail handler")
	}

	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); !ok {
		t.Errorf("bail handler must use tea.Batch (producing tea.BatchMsg), got %T", msg)
	}
}

func TestFormatSessionGoneFlash_ExactSpecWording(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "foo", `session "foo" no longer exists`},
		{"empty", "", `session "" no longer exists`},
		{"with dashes", "my-app-1", `session "my-app-1" no longer exists`},
		{"with spaces", "my app", `session "my app" no longer exists`},
		{"unicode", "名字", `session "名字" no longer exists`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSessionGoneFlash(tc.in)
			if got != tc.want {
				t.Errorf("formatSessionGoneFlash(%q): want %q, got %q", tc.in, tc.want, got)
			}
		})
	}
}
