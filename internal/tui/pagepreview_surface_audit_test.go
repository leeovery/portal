package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is the no-new-surface regression guard for the session
// scrollback preview feature. The spec pins a tight set of permissible
// additions (see § Architecture Summary > No changes to and § Cross-cutting
// Seams > State Package API Reuse):
//
//   - internal/tmux: one new read-only listing method
//     (ListWindowsAndPanesInSession) plus its WindowGroup result type and
//     the unexported field-separator helper. NO new capture wrappers.
//     CapturePane signature unchanged.
//   - internal/state: one new tail-N helper, packaged alongside the
//     existing scrollback writers. Existing writer surface preserved.
//   - internal/restore, cmd/bootstrap, internal/hooks: untouched —
//     specifically no preview tokens.
//   - No new package under internal/ in service of preview.
//
// Each subtest below pins one of those invariants. A failure means
// scope creep crept in via an earlier phase.

// repoRelative resolves a path relative to the repository root from the
// internal/tui working directory. internal/tui sits two directories below
// the repo root, so "internal/tmux/tmux.go" becomes "../../internal/tmux/tmux.go"
// when read from this test's CWD.
func repoRelative(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

// readSourceFiles returns the contents of every non-test .go file under dir,
// keyed by absolute-from-test-cwd path. Test files are excluded so an audit
// test that itself names a forbidden token does not self-trip.
func readSourceFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		out[path] = string(b)
	}
	if len(out) == 0 {
		t.Fatalf("no production .go files found under %s; audit glob is out of date", dir)
	}
	return out
}

// TestSurfaceAudit_TmuxNoNewCaptureWrapper asserts that internal/tmux/tmux.go
// did NOT gain any new capture-wrapper symbol. Per spec § Source of Preview
// Bytes:
//
//	"No new tmux capture wrapper. The existing tmux.Client.CapturePane
//	hardcodes -S - (full scrollback) ... a bounded variant (e.g.
//	CapturePaneTail(target, n)) would have been net-new code. Always-disk
//	avoids that addition entirely."
//
// The forbidden list captures the most plausible names a build phase might
// have introduced. They are scoped to symbol shape ("func (... ) Name(" or
// "func Name(") so a comment that happens to mention the word does not
// trigger.
func TestSurfaceAudit_TmuxNoNewCaptureWrapper(t *testing.T) {
	tmuxPath := repoRelative("internal", "tmux", "tmux.go")
	b, err := os.ReadFile(tmuxPath)
	if err != nil {
		t.Fatalf("read %s: %v", tmuxPath, err)
	}
	src := string(b)

	// Each token is a method-name fragment we expect NOT to see used as a
	// declaration. We scan for "func ... <Name>(" patterns rather than the
	// bare name to avoid false positives in comments. The receiver shape
	// "(c *Client)" is the only declaration shape used in this file for
	// public methods.
	forbiddenSymbols := []string{
		"CapturePaneTail",
		"CapturePaneN",
		"CaptureTail",
		"CapturePaneLastN",
		"CapturePaneRange",
		"CapturePaneBounded",
	}

	for _, sym := range forbiddenSymbols {
		t.Run(sym, func(t *testing.T) {
			// Two declaration shapes are possible: a method on Client or a
			// free function. Either form indicates the surface was added.
			methodForm := "func (c *Client) " + sym + "("
			funcForm := "func " + sym + "("
			if strings.Contains(src, methodForm) || strings.Contains(src, funcForm) {
				t.Errorf(
					"%s declares forbidden capture-wrapper symbol %q; "+
						"per spec § Source of Preview Bytes, preview must not "+
						"introduce new tmux capture wrappers — the read pipeline "+
						"is always-disk via state.ScrollbackFile + tail-N helper.",
					tmuxPath, sym,
				)
			}
		})
	}
}

// TestSurfaceAudit_TmuxCapturePaneSignatureUnchanged pins the existing
// CapturePane method signature. Per spec § Architecture Summary > No
// changes to: "tmux.Client capture path (no new capture wrappers)".
//
// A literal substring assertion on the declaration line is sufficient and
// deliberately rigid — any change to the receiver, name, or parameter list
// must surface here so the reviewer is forced to re-evaluate the
// always-disk decision.
func TestSurfaceAudit_TmuxCapturePaneSignatureUnchanged(t *testing.T) {
	tmuxPath := repoRelative("internal", "tmux", "tmux.go")
	b, err := os.ReadFile(tmuxPath)
	if err != nil {
		t.Fatalf("read %s: %v", tmuxPath, err)
	}
	src := string(b)

	const wantSignature = "func (c *Client) CapturePane(target string) (string, error) {"
	if !strings.Contains(src, wantSignature) {
		t.Errorf(
			"%s no longer contains the verbatim CapturePane signature %q. "+
				"Per spec § Architecture Summary > No changes to, the existing "+
				"capture path must remain untouched.",
			tmuxPath, wantSignature,
		)
	}
}

// TestSurfaceAudit_StateExposesExistingWriters is a lenient existence
// check on the state package's writer surface. Per spec § Cross-cutting
// Seams > State Package API Reuse: "Reused unchanged" — the daemon's
// writer set must remain present after the tail-N addition.
//
// The check is intentionally lenient: it asserts the canonical writers
// are still defined as functions, not that the writer set is exactly
// these and no more. Future legitimate additions must not regress this
// audit.
func TestSurfaceAudit_StateExposesExistingWriters(t *testing.T) {
	stateDir := repoRelative("internal", "state")
	files := readSourceFiles(t, stateDir)

	// Concatenate file contents for a single substring scan — we don't
	// care which file the writer lives in, only that it exists somewhere
	// in the package's production surface.
	var allSrc strings.Builder
	for _, src := range files {
		allSrc.WriteString(src)
		allSrc.WriteByte('\n')
	}
	combined := allSrc.String()

	// Each entry is a function-declaration prefix. Matching the prefix
	// rather than the bare name avoids false positives where the same
	// identifier appears in a doc comment.
	expectedDeclarations := []string{
		"func SetSkeletonMarker(",
		"func UnsetSkeletonMarker(",
		"func WriteScrollbackIfChanged(",
		"func Commit(",
	}

	for _, decl := range expectedDeclarations {
		t.Run(decl, func(t *testing.T) {
			if !strings.Contains(combined, decl) {
				t.Errorf(
					"internal/state no longer declares %q. "+
						"Per spec § Cross-cutting Seams > State Package API Reuse, "+
						"existing writers must remain present alongside the new "+
						"tail-N helper.",
					decl,
				)
			}
		})
	}

	// Triangulate: the new tail-N helper must also be present, since the
	// audit's reason for existing is the addition (Phase 1) plus the
	// preservation of the existing surface. If the helper goes missing,
	// the audit must surface that too.
	const tailDecl = "func TailScrollback("
	if !strings.Contains(combined, tailDecl) {
		t.Errorf(
			"internal/state does not declare %q — the Phase 1 tail-N helper "+
				"is missing. Per spec § Architecture Summary > Read pipeline, "+
				"the helper is the canonical Phase 1 addition.",
			tailDecl,
		)
	}
}

// previewTokens is the set of identifiers that must not appear in
// production source under the named directories. Each token is a symbol
// shape, not a free word, so comments that happen to use the English word
// "preview" don't trigger.
//
// Per spec § Architecture Summary > No changes to: internal/restore,
// cmd/bootstrap, and internal/hooks must remain untouched by preview.
// Any reference to these tokens inside those directories indicates a
// scope-creep regression.
var previewTokens = []string{
	"pagePreview",
	"previewModel",
	"TmuxEnumerator",
	"ScrollbackReader",
}

// auditNoPreviewTokens scans every non-test .go file under dir and fails
// if any previewToken appears as a substring. The check is symbol-shape:
// the tokens are PascalCase identifiers unlikely to collide with
// commentary. The literal lowercase word "preview" is deliberately NOT
// in the list because it can legitimately appear in unrelated comments
// (e.g. "preview the logs"); audits at this level scope to symbol names
// instead.
func auditNoPreviewTokens(t *testing.T, dir string) {
	t.Helper()
	files := readSourceFiles(t, dir)
	for path, src := range files {
		for _, tok := range previewTokens {
			if strings.Contains(src, tok) {
				t.Errorf(
					"%s contains forbidden preview token %q. "+
						"Per spec § Architecture Summary > No changes to, "+
						"this directory must remain untouched by the "+
						"session-scrollback-preview feature.",
					path, tok,
				)
			}
		}
	}
}

// TestSurfaceAudit_RestoreNoPreviewTokens asserts internal/restore source
// files contain no preview-feature symbols.
func TestSurfaceAudit_RestoreNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("internal", "restore"))
}

// TestSurfaceAudit_BootstrapNoPreviewTokens asserts cmd/bootstrap source
// files contain no preview-feature symbols.
func TestSurfaceAudit_BootstrapNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("cmd", "bootstrap"))
}

// TestSurfaceAudit_HooksNoPreviewTokens asserts internal/hooks source
// files contain no preview-feature symbols.
func TestSurfaceAudit_HooksNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("internal", "hooks"))
}

// TestSurfaceAudit_NoNewPackageForPreview asserts no new top-level package
// was added under internal/ in service of preview. Per spec § Architecture
// Summary, the only new code lives in pre-existing packages
// (internal/tui, internal/tmux, internal/state).
//
// The audit is twofold: a directory named "preview" (the obvious shape) is
// rejected outright, and the broader internal/ package set is pinned
// against an allow-list of pre-existing entries known at the start of the
// feature. A new entry not on the list trips the audit.
func TestSurfaceAudit_NoNewPackageForPreview(t *testing.T) {
	internalDir := repoRelative("internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", internalDir, err)
	}

	// Allow-list of pre-existing packages, captured at the start of the
	// session-scrollback-preview feature. A new entry here means the
	// allow-list must be updated deliberately — at which point the
	// reviewer is forced to confirm it is not a preview package.
	preExistingPackages := map[string]struct{}{
		"alias":            {},
		"bootstrapadapter": {},
		"browser":          {},
		"fileutil":         {},
		"fuzzy":            {},
		"hooks":            {},
		"log":              {},
		"logtest":          {},
		"portalbintest":    {},
		"portaltest":       {},
		// prefs: added by the session-tagging-and-grouping feature (mode
		// persistence store); unrelated to scrollback-preview, allow-listed
		// per this audit's own guidance.
		"prefs":         {},
		"project":       {},
		"resolver":      {},
		"restore":       {},
		"restoretest":   {},
		"session":       {},
		"state":         {},
		"statetest":     {},
		"storelog":      {},
		"tmux":          {},
		"tmuxerr":       {},
		"tmuxout":       {},
		"tmuxtest":      {},
		"transienttest": {},
		"tui":           {},
		"ui":            {},
		"warning":       {},
		"xdg":           {},
	}

	// Forbidden names: a directory literally named "preview" is the
	// most likely scope-creep shape. Pinned explicitly so the failure
	// message is specific.
	forbiddenNames := map[string]struct{}{
		"preview":    {},
		"scrollback": {},
		"snapshot":   {},
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if _, forbidden := forbiddenNames[name]; forbidden {
			t.Errorf(
				"new package internal/%s/ exists; per spec "+
					"§ Architecture Summary, the feature must live entirely "+
					"in pre-existing packages (internal/tui, internal/tmux, "+
					"internal/state). Forbidden name pinned by audit.",
				name,
			)
			continue
		}
		if _, ok := preExistingPackages[name]; !ok {
			t.Errorf(
				"new package internal/%s/ exists and is not on the "+
					"pre-existing allow-list. If this addition is unrelated "+
					"to the session-scrollback-preview feature, update the "+
					"audit's preExistingPackages map. Otherwise, per spec "+
					"§ Architecture Summary, preview must not introduce a "+
					"new internal/ package.",
				name,
			)
		}
	}
}

// TestSurfaceAudit_SaveFormatConstantsUnchanged pins the on-disk save format
// for scrollback `.bin` files and hydration FIFOs. Per spec § Architecture
// Summary > No changes to: "Save format or `.bin` file shape" must remain
// untouched. Per § Cross-cutting Seams > State Package API Reuse, preview
// reads via state.ScrollbackFile, so any drift in the path scheme here would
// invisibly redirect preview reads to a non-existent location.
//
// The check pins literal substrings rather than calling the helpers so a
// future refactor that silently swaps the path-construction strategy without
// renaming exported helpers still trips the audit.
func TestSurfaceAudit_SaveFormatConstantsUnchanged(t *testing.T) {
	pathsFile := repoRelative("internal", "state", "paths.go")
	b, err := os.ReadFile(pathsFile)
	if err != nil {
		t.Fatalf("read %s: %v", pathsFile, err)
	}
	src := string(b)

	type pin struct {
		name    string
		literal string
		why     string
	}
	pins := []pin{
		{
			name:    "scrollbackSubdir",
			literal: `scrollbackSubdir  = "scrollback"`,
			why:     "scrollback subdirectory name is part of the save format; preview reads from this exact subdir",
		},
		{
			name:    "ScrollbackFile suffix",
			literal: `paneKey+".bin"`,
			why:     `.bin extension is part of the save-format contract — daemon writes and preview reads must agree`,
		},
		{
			name:    "FIFOPath prefix",
			literal: `"hydrate-"+paneKey+".fifo"`,
			why:     "hydration FIFO naming is part of the save format; the surface audit pins it even though preview itself does not touch FIFOs",
		},
	}

	for _, p := range pins {
		t.Run(p.name, func(t *testing.T) {
			if !strings.Contains(src, p.literal) {
				t.Errorf(
					"%s no longer contains %q (%s); per spec "+
						"§ Architecture Summary > No changes to, the save "+
						"format and `.bin` file shape must remain unchanged.",
					pathsFile, p.literal, p.why,
				)
			}
		})
	}
}
