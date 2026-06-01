package log_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// forbiddenLegacySymbols are the legacy bespoke-logger references that the
// observability migration removed from all PRODUCTION (non-_test.go) source.
// They survive only in internal/state/logger.go (the legacy type itself, kept
// until its dedicated deletion task) and its unit test — both explicitly
// excluded below.
var forbiddenLegacySymbols = []string{
	"state.Component",
	"state.OpenLogger",
	"state.NopLogger",
	"openNoRotateLogger",
}

// excludedFromGuard are the only production files permitted to reference the
// legacy logger symbols: the legacy type's own declaration. (Its unit test is
// excluded by the *_test.go skip below.)
var excludedFromGuard = map[string]bool{
	filepath.Join("internal", "state", "logger.go"): true,
}

// TestNoLegacyLoggerInProductionSource walks every production .go file in the
// repository and fails if any references a forbidden legacy-logger symbol.
// This is the migration's standing guard: the closed observability vocabulary
// is enforced structurally, so a future contributor cannot reintroduce a
// state.Component* / state.OpenLogger / state.NopLogger / openNoRotateLogger
// reference into production code without this test going red.
func TestNoLegacyLoggerInProductionSource(t *testing.T) {
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip VCS, build, and dependency caches.
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
		if excludedFromGuard[rel] {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		content := string(data)
		for _, sym := range forbiddenLegacySymbols {
			if strings.Contains(content, sym) {
				t.Errorf("production source %s references forbidden legacy-logger symbol %q", rel, sym)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk project tree: %v", walkErr)
	}
}
