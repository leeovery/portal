package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestPreviewHermetic_FullLifecycleReadsTheHistoryOnceAtOpen(t *testing.T) {
	history := historyFake(t, strings.Repeat("content\r\n", 40), "work")

	m := openedPreview(t, "work", history, 80, 24)

	keys := []tea.KeyPressMsg{
		{Code: 'k', Text: "k"},
		{Code: 'j', Text: "j"},
		{Code: tea.KeyHome},
		{Code: tea.KeyEnd},
		{Code: tea.KeyLeft},
		{Code: tea.KeyRight},
		{Code: tea.KeyTab},
		keyQuestionMark(),
		keyQuestionMark(),
	}
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Esc, got nil")
	}
	if _, ok := cmd().(previewDismissedMsg); !ok {
		t.Fatalf("Esc cmd produced %T; want previewDismissedMsg", cmd())
	}

	if got := historyCalls(history); len(got) != 1 || got[0] != "work" {
		t.Errorf("History read %v across the lifecycle, want once for work, at open", got)
	}
}

func readAllTUIProductionFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	out := make(map[string]string, len(matches))
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		out[path] = string(data)
	}
	if len(out) == 0 {
		t.Fatalf("expected at least one .go production file in working dir; got 0")
	}
	return out
}

func TestPreviewHermetic_NoHooksDependency(t *testing.T) {
	const forbidden = `"github.com/leeovery/portal/internal/hooks"`
	files := readAllTUIProductionFiles(t)
	for path, src := range files {
		if strings.Contains(src, forbidden) {
			t.Errorf("%s imports forbidden package %s — preview code path must not depend on hooks",
				path, forbidden)
		}
	}
}

// zmxtest drives a real zmx, which runs only in the integration lane: the
// preview's tests stay hermetic, reading history through the seam's fake.
func TestPreviewHermetic_TestFilesDoNotImportZmxtest(t *testing.T) {
	const forbidden = `"github.com/leeovery/portal/internal/zmxtest"`
	var scanned int
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", true) {
		if !strings.HasSuffix(source.Path, "_test.go") {
			continue
		}
		scanned++
		for _, imp := range source.File.Imports {
			if imp.Path.Value == forbidden {
				t.Errorf("%s imports %s; internal/tui's tests must not drive a real zmx", source.Path, forbidden)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no internal/tui test sources scanned; the guard is vacuous")
	}
}
