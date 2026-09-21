package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRelative(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

// Test files are excluded so an audit that itself names a forbidden token does
// not self-trip.
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

func TestSurfaceAudit_TmuxNoNewCaptureWrapper(t *testing.T) {
	tmuxPath := repoRelative("internal", "tmux", "tmux.go")
	b, err := os.ReadFile(tmuxPath)
	if err != nil {
		t.Fatalf("read %s: %v", tmuxPath, err)
	}
	src := string(b)

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
			methodForm := "func (c *Client) " + sym + "("
			funcForm := "func " + sym + "("
			if strings.Contains(src, methodForm) || strings.Contains(src, funcForm) {
				t.Errorf(
					"%s declares forbidden capture-wrapper symbol %q; "+
						"preview must not introduce new tmux capture wrappers — "+
						"the read pipeline is always-disk via "+
						"state.ScrollbackFile + tail-N helper.",
					tmuxPath, sym,
				)
			}
		})
	}
}

func TestSurfaceAudit_TmuxCapturePaneSignatureUnchanged(t *testing.T) {
	tmuxPath := repoRelative("internal", "tmux", "tmux.go")
	b, err := os.ReadFile(tmuxPath)
	if err != nil {
		t.Fatalf("read %s: %v", tmuxPath, err)
	}
	src := string(b)

	const wantSignature = "func (c *Client) CapturePane(target Target) (string, error) {"
	if !strings.Contains(src, wantSignature) {
		t.Errorf(
			"%s no longer contains the verbatim CapturePane signature %q. "+
				"The existing capture path must remain untouched.",
			tmuxPath, wantSignature,
		)
	}
}

func TestSurfaceAudit_StateExposesExistingWriters(t *testing.T) {
	stateDir := repoRelative("internal", "state")
	files := readSourceFiles(t, stateDir)

	var allSrc strings.Builder
	for _, src := range files {
		allSrc.WriteString(src)
		allSrc.WriteByte('\n')
	}
	combined := allSrc.String()

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
						"Existing writers must remain present alongside the new "+
						"tail-N helper.",
					decl,
				)
			}
		})
	}

	const tailDecl = "func TailScrollback("
	if !strings.Contains(combined, tailDecl) {
		t.Errorf(
			"internal/state does not declare %q — the tail-N helper the "+
				"preview read pipeline depends on is missing.",
			tailDecl,
		)
	}
}

// Symbol shapes only — the bare word "preview" would match unrelated prose.
var previewTokens = []string{
	"pagePreview",
	"previewModel",
	"TmuxEnumerator",
	"ScrollbackReader",
}

func auditNoPreviewTokens(t *testing.T, dir string) {
	t.Helper()
	files := readSourceFiles(t, dir)
	for path, src := range files {
		for _, tok := range previewTokens {
			if strings.Contains(src, tok) {
				t.Errorf(
					"%s contains forbidden preview token %q. "+
						"This directory must remain untouched by the "+
						"session-scrollback-preview feature.",
					path, tok,
				)
			}
		}
	}
}

func TestSurfaceAudit_RestoreNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("internal", "restore"))
}

func TestSurfaceAudit_BootstrapNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("cmd", "bootstrap"))
}

func TestSurfaceAudit_HooksNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("internal", "hooks"))
}

func TestSurfaceAudit_NoNewPackageForPreview(t *testing.T) {
	internalDir := repoRelative("internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", internalDir, err)
	}

	preExistingPackages := map[string]struct{}{
		"alias":            {},
		"bootstrapadapter": {},
		"capture":          {},
		"commandertest":    {},
		"fileutil":         {},
		"fuzzy":            {},
		"harnesstest":      {},
		"hooks":            {},
		"hookstest":        {},
		"hooksweep":        {},
		"log":              {},
		"logtest":          {},
		"nanoid":           {},
		"portalbintest":    {},
		"portaltest":       {},
		"prefs":            {},
		"project":          {},
		"resolver":         {},
		"restore":          {},
		"resumemode":       {},
		"restoretest":      {},
		"session":          {},
		"shellquote":       {},
		"sourceguardtest":  {},
		"spawn":            {},
		"spawntest":        {},
		"state":            {},
		"statetest":        {},
		"storelog":         {},
		"theme":            {},
		"themetest":        {},
		"tmux":             {},
		"tmuxerr":          {},
		"tmuxout":          {},
		"tmuxtest":         {},
		"transienttest":    {},
		"tui":              {},
		"warning":          {},
		"xdg":              {},
	}

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
				"new package internal/%s/ exists; the feature must live "+
					"entirely in pre-existing packages (internal/tui, "+
					"internal/tmux, internal/state). Forbidden name pinned "+
					"by audit.",
				name,
			)
			continue
		}
		if _, ok := preExistingPackages[name]; !ok {
			t.Errorf(
				"new package internal/%s/ exists and is not on the "+
					"pre-existing allow-list. If this addition is unrelated "+
					"to the session-scrollback-preview feature, update the "+
					"audit's preExistingPackages map. Otherwise, preview "+
					"must not introduce a new internal/ package.",
				name,
			)
		}
	}
}

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
					"%s no longer contains %q (%s); the save format and "+
						"`.bin` file shape must remain unchanged.",
					pathsFile, p.literal, p.why,
				)
			}
		})
	}
}
