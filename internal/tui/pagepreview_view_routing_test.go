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
	// Wider terminal so the §9.1 chrome cascade lands at tier 1 — the full
	// marker + session + counters + verbose hints render when they all fit.
	m.termWidth = 120

	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}

	rendered := stripANSI(got.View().Content)

	if !strings.Contains(rendered, "Window 1/1") {
		t.Errorf("expected rendered output to contain chrome 'Window 1/1', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Pane 1/1") {
		t.Errorf("expected rendered output to contain chrome 'Pane 1/1', got:\n%s", rendered)
	}
	// §9.1 surfaces the SESSION name in the top bar (the highlighted session,
	// "alpha"), plus the peek-mode marker.
	if !strings.Contains(rendered, "◉ preview") {
		t.Errorf("expected rendered output to contain the '◉ preview' marker, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "alpha") {
		t.Errorf("expected rendered output to contain session name 'alpha', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hello-from-preview") {
		t.Errorf("expected rendered output to contain viewport content 'hello-from-preview', got:\n%s", rendered)
	}

	// Negative assertion: the rendered output must NOT be the sessions list
	// title (which would appear if the View() falls through to the default
	// branch). The §9.1 joined panel's FIRST CONTENT row (after the Vinset gutter
	// rows) is the rounded top border; the header (with the counters) is the row
	// below it. Assert the first content row is the cyan top border, not a
	// sessions-list header.
	contentRows := strings.Split(rendered, "\n")
	if len(contentRows) <= Vinset+1 {
		t.Fatalf("rendered frame has %d rows, fewer than the top gutter + border", len(contentRows))
	}
	topContentRow := strings.TrimSpace(contentRows[Vinset])
	if !strings.HasPrefix(topContentRow, "╭") || !strings.HasSuffix(topContentRow, "╮") {
		t.Errorf("expected first content row to be the preview panel top border ╭…╮, got %q", topContentRow)
	}
	headerContentRow := contentRows[Vinset+1]
	if !strings.Contains(headerContentRow, "Window 1/1") {
		t.Errorf("expected the header content row to carry the preview chrome counters, got %q", headerContentRow)
	}
}
