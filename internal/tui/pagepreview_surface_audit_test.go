package tui

import (
	"go/ast"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
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

// The preview's body is the session's live history, read through the history
// seam: the one History call in the package asks for all of it, rows 0.
func TestSurfaceAudit_PreviewReadsTheWholeHistoryThroughTheSeam(t *testing.T) {
	var calls []string
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		sourceguardtest.ForEachFuncCall(source.File, func(funcName string, call *ast.CallExpr) bool {
			if sourceguardtest.CalleeName(call) != "History" {
				return true
			}
			where := filepath.Base(source.Path) + ":" + funcName
			calls = append(calls, where)
			if len(call.Args) != 3 {
				t.Errorf("%s calls History with %d arguments, want ctx, name and rows", where, len(call.Args))
				return true
			}
			if rows, ok := call.Args[2].(*ast.BasicLit); !ok || rows.Value != strconv.Itoa(0) {
				t.Errorf("%s asks History for a bounded number of rows; the preview shows the whole history, rows 0", where)
			}
			return true
		})
	}
	if len(calls) != 1 || calls[0] != "preview_history.go:body" {
		t.Errorf("History is called from %v, want exactly once, from previewHistory.body in preview_history.go", calls)
	}
}

// The saved scrollback files belong to internal/state, and window and pane
// enumeration to internal/tmux. The preview's files reach neither.
func TestSurfaceAudit_PreviewReadsNoSavedScrollbackAndNoTmux(t *testing.T) {
	forbidden := map[string]string{
		`"github.com/leeovery/portal/internal/state"`: "saved scrollback",
		`"github.com/leeovery/portal/internal/tmux"`:  "tmux",
	}
	files, err := filepath.Glob("*preview*.go")
	if err != nil {
		t.Fatalf("glob preview sources: %v", err)
	}
	var scanned int
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		source := sourceguardtest.PackageSource(t, ".", path)
		for _, imp := range source.File.Imports {
			if what, ok := forbidden[imp.Path.Value]; ok {
				t.Errorf("%s imports %s; the preview reads the session's history, never %s", path, imp.Path.Value, what)
			}
		}
	}
	if scanned == 0 {
		t.Fatalf("no preview production sources matched; the audit scanned nothing")
	}
}

// Symbol shapes only — the bare word "preview" would match unrelated prose.
var previewTokens = []string{
	"pagePreview",
	"previewModel",
	"previewHistory",
	"HistoryReader",
}

func auditNoPreviewTokens(t *testing.T, dir string) {
	t.Helper()
	files := readSourceFiles(t, dir)
	for path, src := range files {
		for _, tok := range previewTokens {
			if strings.Contains(src, tok) {
				t.Errorf(
					"%s contains forbidden preview token %q. "+
						"The preview lives in internal/tui alone.",
					path, tok,
				)
			}
		}
	}
}

func TestSurfaceAudit_StateNoPreviewTokens(t *testing.T) {
	auditNoPreviewTokens(t, repoRelative("internal", "state"))
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
		"alias":           {},
		"attachkeys":      {},
		"backend":         {},
		"backendtest":     {},
		"bootid":          {},
		"capture":         {},
		"commandertest":   {},
		"commitlock":      {},
		"devversion":      {},
		"fileutil":        {},
		"fuzzy":           {},
		"harnesstest":     {},
		"hooks":           {},
		"hookstest":       {},
		"hooksweep":       {},
		"legacyindex":     {},
		"log":             {},
		"logtest":         {},
		"nanoid":          {},
		"portalbintest":   {},
		"portaltest":      {},
		"prefs":           {},
		"proctable":       {},
		"proctest":        {},
		"project":         {},
		"resolver":        {},
		"resumekeys":      {},
		"resumemode":      {},
		"runner":          {},
		"runnerreg":       {},
		"runnertoken":     {},
		"session":         {},
		"sessionindex":    {},
		"shellquote":      {},
		"sourceguardtest": {},
		"spawn":           {},
		"spawntest":       {},
		"state":           {},
		"statetest":       {},
		"storelog":        {},
		"theme":           {},
		"themetest":       {},
		"tmux":            {},
		"tmuxerr":         {},
		"tmuxout":         {},
		"tmuxtest":        {},
		"transienttest":   {},
		"tui":             {},
		"warning":         {},
		"xdg":             {},
		"zmxtest":         {},
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
				"new package internal/%s/ exists; the preview lives in "+
					"internal/tui. Forbidden name pinned by audit.",
				name,
			)
			continue
		}
		if _, ok := preExistingPackages[name]; !ok {
			t.Errorf(
				"new package internal/%s/ exists and is not on the "+
					"pre-existing allow-list. If this addition is unrelated "+
					"to the preview, update the audit's preExistingPackages "+
					"map. Otherwise, the preview must not introduce a new "+
					"internal/ package.",
				name,
			)
		}
	}
}
