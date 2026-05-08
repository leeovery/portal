package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// keyRune builds a single-rune key message — bubbles/list's filter input
// reads runes from msg.Runes regardless of msg.Type, so this helper covers
// every printable character we feed the filter during these tests.
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// keySpaceRune returns the Space keypress shape that bubbletea actually
// produces from the input parser: Type=KeySpace AND Runes=[' ']. The
// Runes slice is what bubbles/textinput inserts via insertRunesFromUserInput,
// so a synthetic Space without Runes does NOT land a literal space — only
// this shape does. (Verified against bubbletea v1.3.10 key.go and bubbles
// v1.0.0 textinput.go.)
func keySpaceRune() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
}

// startFiltering sends "/" to enter filter-input mode on the session list,
// then returns the updated model. Fails the test if SettingFilter() is not
// true after the keypress.
func startFiltering(t *testing.T, m Model) Model {
	t.Helper()
	updatedList, _ := m.sessionList.Update(keyRune('/'))
	m.sessionList = updatedList
	if !m.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==true after pressing /")
	}
	return m
}

// typeFilter sends each rune of s to the model, returning the resulting
// model. Use during SettingFilter() to populate the filter input.
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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

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
	// Edge case: Space typed as the very first character of an empty filter
	// input must still land as a literal space and not open preview.
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(sessions, enum, reader)

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
	// Round-trip: filter → Enter to commit → Space opens preview on the
	// highlighted (matching) session. After commit, SettingFilter() returns
	// false and Space is bound to preview entry.
	sessions := []tmux.Session{
		{Name: "pigeon-fly", Windows: 1, Attached: false},
		{Name: "alpha", Windows: 2, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	m = startFiltering(t, m)
	m = typeFilter(t, m, "pigeon")

	// Enter commits the filter. While SettingFilter() is true, the
	// updateSessionList Space short-circuit forwards Enter to bubbles/list,
	// which transitions filterState to FilterApplied.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==false after Enter, got true")
	}

	// Sanity: pigeon-fly is the highlighted match.
	si, ok := got.selectedSessionItem()
	if !ok {
		t.Fatalf("expected a highlighted item after committed filter")
	}
	if si.Session.Name != "pigeon-fly" {
		t.Fatalf("expected highlighted match to be %q, got %q", "pigeon-fly", si.Session.Name)
	}

	// Now Space must open preview on the highlighted match.
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

// TestExactlyOneSpaceBranchInUpdateSessionList enforces the spec invariant:
// "There is no second binding for 'open preview' while filtering." The
// updateSessionList function must contain exactly ONE place where
// msg.Type == tea.KeySpace is matched. A second branch — even a guarded
// "open preview while filtering" path — would contradict the spec
// (§Filter Behaviour with Preview) and re-introduce magic-Space behaviour.
//
// This is a static/code-inspection assertion: it scans model.go's
// updateSessionList body for occurrences of tea.KeySpace and asserts a
// count of exactly one. If a future change introduces a second Space
// branch, this test fires immediately and points back to the spec.
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

// extractFuncBody returns the body of the named method on Model, or "" if
// not found. It uses a brace-balanced scan starting at the function
// signature, which is sufficient for the in-file lookup performed here.
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
