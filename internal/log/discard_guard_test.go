package log_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const forbiddenDiscardConstruction = "slog.NewTextHandler(io.Discard"

func TestNoDiscardLoggerConstruction(t *testing.T) {
	scan := scanForDiscardConstruction(t)

	for _, finding := range scan.findings {
		t.Errorf("%s constructs a discard-backed *slog.Logger; route through log.OrDiscard / log.Discard instead", finding)
	}
}

func TestDiscardGuard_Rule(t *testing.T) {
	const openCoded = `package fixture

import (
	"io"
	"log/slog"
)

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))
`
	const clean = "package fixture\n"

	for _, tc := range []struct {
		name         string
		files        map[string]string
		wantScanned  int
		wantFindings []string
	}{
		{
			name:         "it fails for an open-coded discard handler in a test file",
			files:        map[string]string{filepath.Join("internal", "tui", "paint_test.go"): openCoded},
			wantScanned:  1,
			wantFindings: []string{filepath.Join("internal", "tui", "paint_test.go")},
		},
		{
			name: "it exempts internal/log's own in-package tests",
			files: map[string]string{
				filepath.Join("internal", "log", "rotate_test.go"): openCoded,
				filepath.Join("internal", "tui", "paint_test.go"):  clean,
			},
			wantScanned: 1,
		},
		{
			name: "it exempts internal/prefs for its stated reason",
			files: map[string]string{
				filepath.Join("internal", "prefs", "store.go"):    openCoded,
				filepath.Join("internal", "tui", "paint_test.go"): clean,
			},
			wantScanned: 1,
		},
		{
			// prefs' leaf rule binds its production imports alone, so its test
			// files keep the route and stay under the rule.
			name:         "it covers internal/prefs test files, which may reach internal/log",
			files:        map[string]string{filepath.Join("internal", "prefs", "store_test.go"): openCoded},
			wantScanned:  1,
			wantFindings: []string{filepath.Join("internal", "prefs", "store_test.go")},
		},
		{
			name: "it enumerates through the shared repo walk",
			files: map[string]string{
				filepath.Join("vendor", "dep", "vendored.go"):      openCoded,
				filepath.Join(".worktrees", "old", "stale.go"):     openCoded,
				filepath.Join("node_modules", "pkg", "bundle.go"):  openCoded,
				filepath.Join("internal", "tui", "nested_test.go"): clean,
			},
			wantScanned: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageDiscardGuardTree(t, tc.files)

			scan := scanForDiscardConstruction(t, sourceguardtest.Rooted(root))

			if scan.scanned != tc.wantScanned {
				t.Fatalf("scanned %d files, want %d", scan.scanned, tc.wantScanned)
			}
			if !slices.Equal(scan.findings, tc.wantFindings) {
				t.Errorf("findings = %v, want %v", scan.findings, tc.wantFindings)
			}
		})
	}
}

// A guard whose scan came back empty would report a clean tree forever.
func TestDiscardGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanForDiscardConstruction(recorder, sourceguardtest.Rooted(t.TempDir()))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("scan of a tree holding no sources reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
	if !strings.Contains(recorder.Fatals[0], "stopped looking") {
		t.Errorf("fatal message %q does not say the guard would pass having stopped looking", recorder.Fatals[0])
	}
}

type discardScan struct {
	scanned  int
	findings []string
}

// scanForDiscardConstruction reads every .go file the shared repo walk offers —
// production and test alike — and reports the unexempted ones that construct a
// discard-backed handler by hand.
func scanForDiscardConstruction(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) discardScan {
	t.Helper()

	root, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources, opts...)

	var scan discardScan
	for _, source := range sources {
		if discardConstructionExempt(source.Path) {
			continue
		}
		scan.scanned++
		data, err := os.ReadFile(filepath.Join(root, source.Path))
		if err != nil {
			t.Fatalf("read %s: %v", source.Path, err)
			return scan
		}
		if strings.Contains(string(data), forbiddenDiscardConstruction) {
			scan.findings = append(scan.findings, source.Path)
		}
	}
	return scan
}

// discardConstructionExempt reports the files that may hand-roll the
// construction. discard.go is that route's own home, and this guard file names
// the forbidden construction verbatim as its own subject, so internal/log is
// exempted as one directory-wide rule rather than by singling that file out.
// internal/prefs has no route at all: it is a leaf whose production code must
// not import internal/log — a constraint that binds its imports alone, so its
// test files keep the route and stay under the rule.
func discardConstructionExempt(path string) bool {
	dir, file := filepath.Split(path)
	dir = filepath.Clean(dir)
	isTest := strings.HasSuffix(file, "_test.go")

	switch dir {
	case filepath.Join("internal", "log"):
		return isTest || file == "discard.go"
	case filepath.Join("internal", "prefs"):
		return !isTest
	}
	return false
}

func stageDiscardGuardTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
