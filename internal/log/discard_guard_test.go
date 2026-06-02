package log_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// forbiddenDiscardConstruction is the literal fragment of a discard-backed
// *slog.Logger construction. Per the spec's "Call-site logging pattern"
// Prohibited rule ("Direct construction of *slog.Logger outside the
// internal/log package"), the single canonical io.Discard sink lives only in
// internal/log/discard.go; every nil-tolerant fallback site routes through
// log.OrDiscard / log.Discard instead of re-declaring its own.
const forbiddenDiscardConstruction = "slog.NewTextHandler(io.Discard"

// discardConstructionAllowed is the only production file permitted to construct
// the discard sink: internal/log's own declaration.
var discardConstructionAllowed = map[string]bool{
	filepath.Join("internal", "log", "discard.go"): true,
}

// TestNoDiscardLoggerConstructionInProductionSource walks every production
// .go file in the repository and fails if any (outside internal/log/discard.go)
// constructs a discard-backed *slog.Logger. This is the standing guard for the
// consolidation: a future contributor cannot reintroduce a per-package
// discardLogger declaration without this test going red, eliminating the
// drift-with-no-compiler-signal risk that motivated the consolidation.
func TestNoDiscardLoggerConstructionInProductionSource(t *testing.T) {
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if discardConstructionAllowed[rel] {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), forbiddenDiscardConstruction) {
			t.Errorf("production source %s constructs a discard-backed *slog.Logger; route through log.OrDiscard / log.Discard instead", rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk project tree: %v", walkErr)
	}
}
