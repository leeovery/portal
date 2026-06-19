package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestModelViewRoutesPagePreviewToPreviewModel pins the integration that the
// top-level Model.View() must route pagePreview to m.preview.View(). Without
// the case arm, View() falls through to the default branch and renders the
// session list — invisible to the user despite Update routing correctly.
//
// Regression: cycle 1 of analysis missed this because every preview view test
// calls previewModel.View() directly rather than driving through Model.View()
// while activePage == pagePreview.
func TestModelViewRoutesPagePreviewToPreviewModel(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello-from-preview\n")}
	m := modelWithSeams(sessions, enum, reader)
	// Wider terminal so the chrome cascade lands at tier 1 — the assertions
	// below check for the full "win: editor" segment which only renders when
	// the full verbose chrome (counters + segment + verbose keymap) fits.
	m.termWidth = 120

	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}

	rendered := got.View().Content

	if !strings.Contains(rendered, "Window 1 of 1") {
		t.Errorf("expected rendered output to contain chrome 'Window 1 of 1', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Pane 1 of 1") {
		t.Errorf("expected rendered output to contain chrome 'Pane 1 of 1', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "editor") {
		t.Errorf("expected rendered output to contain window name 'editor', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hello-from-preview") {
		t.Errorf("expected rendered output to contain viewport content 'hello-from-preview', got:\n%s", rendered)
	}

	// Negative assertion: the rendered output must NOT be the sessions list
	// title (which would appear if the View() falls through to the default
	// branch). The default sessions title contains "session" or the list's
	// own header — assert the chrome line is the FIRST line of output, not
	// a sessions-list header.
	topRow := firstLine(rendered)
	if !strings.Contains(topRow, "Window 1 of 1") {
		t.Errorf("expected first line to be the preview chrome, got %q", topRow)
	}
}
