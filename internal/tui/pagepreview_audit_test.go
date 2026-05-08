package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreviewLayerAudit_NoPortalSaverReferences is a static audit that
// pins the spec invariant from § Cross-cutting Seams > _portal-saver
// Self-Reference and § Out of Scope (v1) > "Preview-layer _portal-saver
// suppression":
//
//	The preview layer must NOT contain any name-based suppression of
//	_portal-saver. Exclusion is the responsibility of the Sessions-list
//	source (Client.ListSessions in internal/tmux/tmux.go), which strips
//	any session whose name starts with "_".
//
// Adding a string match on "_portal-saver" inside a preview source file
// would either (a) duplicate the canonical filter or (b) silently mask a
// regression where the source filter was removed. Either case must fail
// loudly. The audit walks the on-disk preview files (production source
// only — test files are excluded so the test itself can mention the
// constant in assertions) and asserts each is free of the literal.
func TestPreviewLayerAudit_NoPortalSaverReferences(t *testing.T) {
	const forbidden = "_portal-saver"

	// Glob-walk the package directory, scoping to preview source files
	// only. The patterns mirror the spec's preview-layer file shape:
	// pagepreview*.go (page state machine + per-arm code) and
	// preview_*.go (seams + adapter wiring). Test files are excluded so
	// this test's own constant references do not self-trip.
	patterns := []string{"pagepreview*.go", "preview_*.go"}
	var sourceFiles []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %q: %v", pattern, err)
		}
		for _, path := range matches {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			sourceFiles = append(sourceFiles, path)
		}
	}

	if len(sourceFiles) == 0 {
		t.Fatalf("preview-layer audit found zero source files; glob patterns are out of date with the package layout")
	}

	for _, path := range sourceFiles {
		t.Run(path, func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(contents), forbidden) {
				t.Errorf(
					"preview source file %s mentions %q; "+
						"per spec § Cross-cutting Seams > _portal-saver Self-Reference, "+
						"the preview layer must not introduce a name-based blacklist. "+
						"Exclusion belongs to the Sessions-list source (internal/tmux Client.ListSessions).",
					path, forbidden,
				)
			}
		})
	}
}

// TestPreviewLayerAudit_ExclusionAppliedAtSource_NotPreviewLayer
// triangulates the audit: it asserts both halves of the spec invariant
// in a single test so a future reader sees the audit recorded as a
// proven property rather than two unrelated test cases.
//
//  1. The canonical Sessions-list source — the file path
//     internal/tmux/tmux.go relative to the repo root — DOES contain the
//     filter logic for underscore-prefixed sessions. (Pinned by literal
//     substring on the filter comment so a future refactor that strips
//     the filter is forced to also rewrite the comment, surfacing intent.)
//  2. The preview model file — internal/tui/pagepreview.go — does NOT
//     mention _portal-saver. (Subset of the broader audit above; pinned
//     here independently so the named-file invariant is its own test.)
func TestPreviewLayerAudit_ExclusionAppliedAtSource_NotPreviewLayer(t *testing.T) {
	const forbidden = "_portal-saver"

	previewModel := "pagepreview.go"
	previewBytes, err := os.ReadFile(previewModel)
	if err != nil {
		t.Fatalf("read %s: %v", previewModel, err)
	}
	if strings.Contains(string(previewBytes), forbidden) {
		t.Errorf("preview model file %s mentions %q; preview must not introduce name-based suppression", previewModel, forbidden)
	}

	// internal/tui sits at <repo>/internal/tui, so the canonical filter
	// source is two directories up plus internal/tmux/tmux.go.
	listSource := filepath.Join("..", "tmux", "tmux.go")
	listBytes, err := os.ReadFile(listSource)
	if err != nil {
		t.Fatalf("read %s: %v", listSource, err)
	}
	listSrc := string(listBytes)

	// Pin the filter's existence by structural markers (the canonical
	// HasPrefix("_") check inside ListSessions). If a future refactor
	// removes or relocates the filter, this assertion fails — and the
	// reviewer is forced to either re-introduce the filter or relocate
	// this audit to the new canonical site.
	if !strings.Contains(listSrc, `strings.HasPrefix(s.Name, "_")`) {
		t.Errorf(
			"canonical Sessions-list filter at %s no longer contains "+
				`strings.HasPrefix(s.Name, "_") — the underscore-prefix `+
				"exclusion has been removed or relocated. Per spec "+
				"§ Cross-cutting Seams > _portal-saver Self-Reference, "+
				"exclusion must remain at the list-population source.",
			listSource,
		)
	}
}
