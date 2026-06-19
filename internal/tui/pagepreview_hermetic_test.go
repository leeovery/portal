package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Hermetic side-effect-free contract (Phase 4 task 4-8):
//
// Pin the spec invariant from § Overview > Side-effect-free contract,
// § Acceptance Criteria > Side-effect-free contract, and § Architecture
// Summary > Wiring shape:
//
//   - Opening and dismissing preview leaves session state byte-identical:
//     no hydration, no resume-hook firing, no tmux marker mutation, no FIFO
//     consumed.
//   - The preview code path issues exactly one TmuxEnumerator call (the
//     structural enumeration at open) across the full lifecycle.
//   - The preview code path issues only ScrollbackReader.Tail calls — one
//     per focus event — and no other I/O on .bin paths.
//   - The preview code path makes zero calls into hooks.Store, no writes
//     via state-package writers, and no FIFO creation or drain.
//
// The first subtest exercises a full lifecycle against mocked seams and
// audits call counts. The remaining subtests are static-import audits
// over the source tree to pin the absence of forbidden imports/symbols
// — they are the operationalisation of "no writes" in the absence of
// a runtime side-effect snapshot.
//
// No production code changes — these tests are regression-pinning only.

// hermeticEnumerator is a recording TmuxEnumerator returning a 2-window x
// 2-pane shape. The 2x2 shape ensures every keypress in the lifecycle
// sequence (Tab, Tab, ], Tab, Tab, [) is non-degenerate — i.e. each one
// triggers a read. Degenerate (1x1, 1xN, Nx1) shapes would silently
// no-op some keypresses and the per-focus read budget would not match.
type hermeticEnumerator struct {
	calls   int
	lastArg string
}

func (e *hermeticEnumerator) ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) {
	e.calls++
	e.lastArg = session
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
	}, nil
}

// hermeticReader is a recording ScrollbackReader returning a non-empty
// bytes slice on every Tail call. The bytes value is irrelevant to the
// hermetic contract — only the call count is asserted.
type hermeticReader struct {
	calls []string
}

func (r *hermeticReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	return []byte("content"), nil
}

func TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads(t *testing.T) {
	// Lifecycle: construct → Tab, Tab, ], Tab, Tab, [, Esc → dismiss.
	// Across a 2-window x 2-pane fixture every cycle keypress moves focus
	// (no degenerate no-ops), so each one triggers exactly one Tail call.
	// Esc emits previewDismissedMsg and triggers no read.
	enum := &hermeticEnumerator{}
	reader := &hermeticReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	keys := []tea.KeyPressMsg{
		{Code: tea.KeyTab},     // (0,0) → (0,1)
		{Code: tea.KeyTab},     // (0,1) → (0,0) wrap
		{Code: ']', Text: "]"}, // → (1,0)
		{Code: tea.KeyTab},     // (1,0) → (1,1)
		{Code: tea.KeyTab},     // (1,1) → (1,0) wrap
		{Code: '[', Text: "["}, // → (0,0)
	}
	for _, k := range keys {
		m, _ = m.Update(k)
	}

	// Esc emits previewDismissedMsg — verify both that the cmd is non-nil
	// and that the dispatched message is the dismiss shape. Esc must not
	// trigger any Tail or enumerator call.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Esc, got nil")
	}
	if _, ok := cmd().(previewDismissedMsg); !ok {
		t.Fatalf("Esc cmd produced %T; want previewDismissedMsg", cmd())
	}

	// Enumerator: exactly one call across the full lifecycle, with the
	// session name flowing from NewPreviewModel's first argument verbatim.
	if enum.calls != 1 {
		t.Errorf("expected ListWindowsAndPanesInSession called exactly 1 time across full lifecycle, got %d",
			enum.calls)
	}
	if enum.lastArg != "work" {
		t.Errorf("enumerator received session %q; want %q (constructor must pass the session arg through verbatim)",
			enum.lastArg, "work")
	}

	// Reader: exactly 1 (open) + 6 (cycle keypresses) = 7 calls.
	// Esc is the 7th keystroke in the lifecycle but doesn't read.
	const wantReads = 1 + 6
	if len(reader.calls) != wantReads {
		t.Errorf("expected %d Tail calls (1 open + 6 cycle keypresses; Esc reads 0), got %d (calls=%v)",
			wantReads, len(reader.calls), reader.calls)
	}
}

// readPagePreviewFiles returns the concatenated contents of every
// pagepreview*.go file in internal/tui (production sources only — test
// files matching pagepreview*_test.go are excluded). The audit subtests
// scope FIFO and writer-symbol scans to this set so unrelated state /
// FIFO code paths in model.go (or anywhere else in the tui package) do
// not produce false positives.
func readPagePreviewFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob("pagepreview*.go")
	if err != nil {
		t.Fatalf("glob pagepreview*.go: %v", err)
	}
	out := make(map[string]string, len(matches))
	for _, path := range matches {
		// Exclude test files — the static audit pins the production
		// surface (and the hermetic test itself; that file is read but
		// only checked alongside the others — its own forbidden-symbol
		// matches are guarded by the precise import-path/symbol shape
		// used below).
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

// readAllTUIProductionFiles returns the concatenated contents of every
// .go file in internal/tui that is NOT a _test.go file. Used by the
// import audits that must scope to the entire tui package surface
// (no-hooks-import, no-state-writers).
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
	// The internal/tui package must not import the hooks package at all —
	// hook firing is exclusively driven by the hydrate helper's exec
	// chain (per CLAUDE.md > Resume hooks), and preview is a read-only
	// page that never mutates state. A drift here (e.g. someone wires a
	// hooks.Store into the preview adapter) is a side-effect contract
	// violation.
	//
	// Match on the canonical import path string rather than a loose
	// "hooks" substring — comments and identifiers may legitimately
	// contain the word "hooks" without implying a real dependency.
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
	// Preview consumes only read-side helpers from state — TailScrollback,
	// ScrollbackFile, SanitizePaneKey, Dir. Any writer symbol from state
	// in tui production code would prove a side-effect leak from the
	// preview path (or from any other tui code path that could be reached
	// by a future preview refactor).
	//
	// Symbols are matched as `state.<Symbol>` qualified references rather
	// than bare substrings, so commentary mentioning these names without
	// the qualifier does not trip the audit.
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
	// Preview never creates, drains, or names a FIFO — the read pipeline
	// is always-disk via TailScrollback (per § Source of Preview Bytes).
	// model.go and other tui production files may legitimately reference
	// FIFO in non-preview code paths (e.g. restore plumbing wired through
	// the bootstrap orchestrator), so the scan is scoped strictly to
	// pagepreview*.go production files.
	//
	// Match both case forms ("FIFO" and "fifo") to cover identifier and
	// comment usage. Any hit is a contract violation.
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
	// The hermetic contract requires test files to not depend on real-tmux
	// fixtures or shared restore drivers. Tests must mock the seams
	// directly and exercise Update via synthetic tea.KeyMsg values per the
	// project test convention.
	//
	// The forbidden import paths are built up at runtime from short
	// fragments so that the literal contiguous strings do not appear in
	// this file's source — otherwise the audit would self-trip on its
	// own data.
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
