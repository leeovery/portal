package tui

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
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

// The saved scrollback files belong to internal/state, which the preview's
// files never reach.
func TestSurfaceAudit_PreviewReadsNoSavedScrollback(t *testing.T) {
	forbidden := map[string]string{
		`"github.com/leeovery/portal/internal/state"`: "saved scrollback",
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
	t.Run("it finds no package named for the preview under internal", func(t *testing.T) {
		for _, found := range previewPackages(t, repoRelative("internal")) {
			t.Errorf("new package internal/%s/ exists; the preview lives in internal/tui", found)
		}
	})

	t.Run("it reports each package named for the preview", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"preview", "scrollback", "snapshot", "tui", "previews"} {
			if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatalf("stage %s: %v", name, err)
			}
		}
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = previewPackages(&rec, dir) })
		if want := []string{"preview", "scrollback", "snapshot"}; rec.Failed() || !slices.Equal(found, want) {
			t.Errorf("previewPackages reported %q and %s, want %q", found, rec.Report(), want)
		}
	})

	t.Run("it fails on a directory holding no package", func(t *testing.T) {
		var rec harnesstest.Recorder
		rec.Run(func() { previewPackages(&rec, t.TempDir()) })
		if len(rec.Fatals) != 1 {
			t.Errorf("previewPackages over an empty directory reported %s, want one fatal", rec.Report())
		}
	})
}

// previewPackageNames are the names a package split out for the preview
// would carry.
var previewPackageNames = []string{"preview", "scrollback", "snapshot"}

// previewPackages returns, in directory order, each package under dir named
// for the preview. A dir holding no package at all is fatal: the audit would
// pass by having stopped looking.
func previewPackages(t harnesstest.TestingT, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
		return nil
	}
	var packages int
	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		packages++
		if slices.Contains(previewPackageNames, e.Name()) {
			found = append(found, e.Name())
		}
	}
	if packages == 0 {
		t.Fatalf("no package under %s; the audit scanned nothing", dir)
		return nil
	}
	return found
}
