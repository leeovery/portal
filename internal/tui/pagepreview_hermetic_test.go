package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

type hermeticEnumerator struct {
	calls   int
	lastArg string
}

func (e *hermeticEnumerator) ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) {
	e.calls++
	e.lastArg = session
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1, Token: "abcdef", Pending: true}}},
	}, nil
}

type hermeticReader struct {
	calls []string
}

func (r *hermeticReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
	r.calls = append(r.calls, paneKey)
	return []byte("content"), nil
}

func TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads(t *testing.T) {
	enum := &hermeticEnumerator{}
	reader := &hermeticReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	keys := []tea.KeyPressMsg{
		nextPaneKey,
		nextPaneKey,
		nextWindowKey,
		nextPaneKey,
		nextPaneKey,
		prevWindowKey,
	}
	for _, k := range keys {
		m, _ = m.Update(k)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Esc, got nil")
	}
	if _, ok := cmd().(previewDismissedMsg); !ok {
		t.Fatalf("Esc cmd produced %T; want previewDismissedMsg", cmd())
	}

	if enum.calls != 1 {
		t.Errorf("expected ListWindowsAndPanesInSession called exactly 1 time across full lifecycle, got %d",
			enum.calls)
	}
	if enum.lastArg != "work" {
		t.Errorf("enumerator received session %q; want %q (constructor must pass the session arg through verbatim)",
			enum.lastArg, "work")
	}

	const wantReads = 1 + 6
	if len(reader.calls) != wantReads {
		t.Errorf("expected %d Tail calls (1 open + 6 cycle keypresses; Esc reads 0), got %d (calls=%v)",
			wantReads, len(reader.calls), reader.calls)
	}
}

func readPagePreviewFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob("pagepreview*.go")
	if err != nil {
		t.Fatalf("glob pagepreview*.go: %v", err)
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
		t.Fatalf("expected at least one pagepreview*.go production file in working dir; got 0")
	}
	return out
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

func TestPreviewHermetic_NoStatePackageWriters(t *testing.T) {
	forbiddenSymbols := []string{
		"state.SetSkeletonMarker",
		"state.UnsetSkeletonMarker",
		"state.UnsetSkeletonMarkerForFIFO",
		"state.WriteScrollbackIfChanged",
		"state.CaptureAndHashPane",
		"state.CaptureStructure",
		"state.SeedHashMap",
		"state.Commit",
		"state.BootstrapPortalSaver",
		"state.EnsurePortalSaverVersion",
	}
	files := readAllTUIProductionFiles(t)
	for path, src := range files {
		for _, sym := range forbiddenSymbols {
			if strings.Contains(src, sym) {
				t.Errorf("%s references forbidden state writer %s — preview code path must not call state writers",
					path, sym)
			}
		}
	}
}

func TestPreviewHermetic_NoFIFOReferences(t *testing.T) {
	files := readPagePreviewFiles(t)
	needles := []string{"FIFO", "fifo"}
	for path, src := range files {
		for _, needle := range needles {
			if strings.Contains(src, needle) {
				t.Errorf("%s references forbidden token %q — preview pipeline must not touch FIFOs",
					path, needle)
			}
		}
	}
}

func TestPreviewHermetic_TestFilesDoNotImportTmuxtestOrRestoretest(t *testing.T) {
	// Assembled from fragments so the forbidden paths never appear contiguously
	// here — this file is one of the files scanned, and would self-trip.
	const importPathPrefix = `"github.com/leeovery/portal/internal/`
	forbiddenImports := []string{
		importPathPrefix + "tmux" + `test"`,
		importPathPrefix + "restore" + `test"`,
	}

	matches, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob *_test.go: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one *_test.go file in working dir; got 0")
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := string(data)
		for _, imp := range forbiddenImports {
			if strings.Contains(src, imp) {
				t.Errorf("%s imports forbidden test-only package %s — internal/tui tests must not depend on real-tmux or restore drivers",
					path, imp)
			}
		}
	}
}
