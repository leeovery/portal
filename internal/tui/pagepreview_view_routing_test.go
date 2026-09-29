package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestModelViewRoutesPagePreviewToPreviewModel(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello-from-preview\n")}
	m := modelWithSeams(t, sessions, enum, reader)
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
	if !strings.Contains(rendered, "◉ preview") {
		t.Errorf("expected rendered output to contain the '◉ preview' marker, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "alpha") {
		t.Errorf("expected rendered output to contain session name 'alpha', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hello-from-preview") {
		t.Errorf("expected rendered output to contain viewport content 'hello-from-preview', got:\n%s", rendered)
	}

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
