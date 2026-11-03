package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "foo")

	want := `session "foo" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_FlashUsesLiteralDoubleQuotesAroundName(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	if !strings.Contains(got.flashText, `"alpha"`) {
		t.Errorf("flash must contain literal-double-quoted name `\"alpha\"`, got %q", got.flashText)
	}
}

func TestPreviewAttachBail_FlashHasNoTrailingPunctuation(t *testing.T) {
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, "foo")

	last := got.flashText[len(got.flashText)-1]
	if last == '.' || last == '!' || last == '?' || last == ',' || last == ';' || last == ':' {
		t.Errorf("flashText must not end with punctuation, got %q (last byte %q)", got.flashText, string(last))
	}
	if got.flashText != `session "foo" no longer exists` {
		t.Errorf("flashText must equal exact spec wording, got %q", got.flashText)
	}
}

func TestPreviewAttachBail_BumpsFlashGen(t *testing.T) {
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	if m.flashGen != 0 {
		t.Fatalf("setup invariant: want flashGen=0, got %d", m.flashGen)
	}

	got, _ := pressSpaceThenBail(t, m, "foo")

	if got.flashGen != 1 {
		t.Errorf("flashGen after bail: want 1, got %d", got.flashGen)
	}
}

func TestPreviewAttachBail_ReturnsBatchWithRefreshAndTick(t *testing.T) {
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

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
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

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
	sessions := []Session{
		{Name: "foo", Answered: true, Attached: false},
		{Name: "bar", Answered: true, Attached: false},
	}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	postKill := []Session{{Name: "bar", Answered: true, Attached: false}}
	lister := &stepListerStub{steps: [][]Session{postKill}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)
	m.termWidth = 80
	m.termHeight = 24

	got, _ := pressSpaceThenBail(t, m, "foo")

	rendered := got.View().Content
	want := `session "foo" no longer exists`
	if !strings.Contains(rendered, want) {
		t.Errorf("rendered View must contain flash %q before refresh resolves, got:\n%s", want, rendered)
	}
}

func TestPreviewAttachBail_SpecialCharsInNamePreservedVerbatim(t *testing.T) {
	const weird = "my session-1.2 名字"
	sessions := []Session{{Name: weird, Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, _ := pressSpaceThenBail(t, m, weird)

	want := `session "` + weird + `" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText with special chars: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_EmptySessionNameDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("bail with empty session panicked: %v", r)
		}
	}()

	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "")
	if got.activePage != PageSessions {
		t.Errorf("expected PageSessions after bail with empty name, got %v", got.activePage)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd even with empty session")
	}
	want := `session "" no longer exists`
	if got.flashText != want {
		t.Errorf("flashText with empty name: want %q, got %q", want, got.flashText)
	}
}

func TestPreviewAttachBail_BailHandlerNotUsingTeaSequence(t *testing.T) {
	sessions := []Session{{Name: "foo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

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
