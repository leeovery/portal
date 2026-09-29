package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keySpaceRune() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

func startFiltering(t *testing.T, m Model) Model {
	t.Helper()
	updatedList, _ := m.sessionList.Update(keyRune('/'))
	m.sessionList = updatedList
	if !m.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==true after pressing /")
	}
	return m
}

func typeFilter(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		updated, _ := m.Update(keyRune(r))
		got, ok := updated.(Model)
		if !ok {
			t.Fatalf("expected Model, got %T", updated)
		}
		m = got
	}
	return m
}

func TestSpaceDuringSettingFilterInsertsLiteralSpaceIntoFilterValue(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "pigeon-fly", Windows: 1, Attached: false},
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	m = startFiltering(t, m)
	m = typeFilter(t, m, "pigeon")

	updated, _ := m.Update(keySpaceRune())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}

	if want, have := "pigeon ", got.sessionList.FilterValue(); have != want {
		t.Errorf("FilterValue after Space: want %q, got %q", want, have)
	}
	if got.activePage == pagePreview {
		t.Errorf("activePage must NOT be pagePreview while SettingFilter, got pagePreview")
	}
	if enum.calls != 0 {
		t.Errorf("expected NewPreviewModel NOT called while SettingFilter, got enumerator.calls=%d", enum.calls)
	}
}

func TestSpaceDuringSettingFilterDoesNotChangeActivePage(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "pigeon-fly", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	m = startFiltering(t, m)
	m = typeFilter(t, m, "pigeon")

	updated, _ := m.Update(keySpaceRune())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}

	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions while SettingFilter, got %v", got.activePage)
	}
}

func TestSpaceAtStartOfFilterInputPassesThroughAsLiteralSpace(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, sessions, enum, reader)

	m = startFiltering(t, m)

	updated, _ := m.Update(keySpaceRune())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}

	if want, have := " ", got.sessionList.FilterValue(); have != want {
		t.Errorf("FilterValue after leading Space: want %q, got %q", want, have)
	}
	if got.activePage == pagePreview {
		t.Errorf("activePage must NOT be pagePreview, got pagePreview")
	}
	if enum.calls != 0 {
		t.Errorf("expected NewPreviewModel NOT called, got enumerator.calls=%d", enum.calls)
	}
}

func TestSpaceAfterEnterCommitOpensPreviewOnHighlightedMatch(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "pigeon-fly", Windows: 1, Attached: false},
		{Name: "alpha", Windows: 2, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	m = startFiltering(t, m)
	m = typeFilter(t, m, "pigeon")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==false after Enter, got true")
	}

	si, ok := got.selectedSessionItem()
	if !ok {
		t.Fatalf("expected a highlighted item after committed filter")
	}
	if si.Session.Name != "pigeon-fly" {
		t.Fatalf("expected highlighted match to be %q, got %q", "pigeon-fly", si.Session.Name)
	}

	updated, _ = got.Update(keySpaceRune())
	got2, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got2.activePage != pagePreview {
		t.Errorf("expected activePage=pagePreview after Enter-commit + Space, got %v", got2.activePage)
	}
	if enum.calls != 1 {
		t.Errorf("expected NewPreviewModel called once after Enter-commit + Space, got enumerator.calls=%d", enum.calls)
	}
	if enum.lastArg != "pigeon-fly" {
		t.Errorf("expected enumerator called for highlighted match %q, got %q", "pigeon-fly", enum.lastArg)
	}
}

func TestExactlyOneSpaceBranchInUpdateSessionList(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("read model.go: %v", err)
	}

	body := extractFuncBody(string(src), "updateSessionList")
	if body == "" {
		t.Fatalf("could not locate updateSessionList in model.go")
	}

	count := strings.Count(body, "tea.KeySpace")
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of tea.KeySpace in updateSessionList (single Space binding invariant), got %d", count)
	}
}

func extractFuncBody(src, name string) string {
	signature := "func (m Model) " + name + "("
	idx := strings.Index(src, signature)
	if idx < 0 {
		return ""
	}
	openBrace := strings.Index(src[idx:], "{")
	if openBrace < 0 {
		return ""
	}
	start := idx + openBrace
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	return ""
}
