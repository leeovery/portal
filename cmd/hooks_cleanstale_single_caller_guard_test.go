// Tests in this file inspect on-disk source and MUST NOT use t.Parallel.
package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestHooksCleanStale_NoBootstrapStepIsAnAutomaticCaller is the AC5 structural
// guard (spec § Daemon-Owned Hooks Cleanup → Decision; § Test Strategy → Branch
// selection: "assert the daemon's throttled cleanup is the only hooks-CleanStale
// caller left"). Phase 1 removed the hooks CleanStale step from the orchestrator
// entirely, so after this feature the daemon's maybeRunHookCleanup and the
// manual `portal clean` path (cleanCmd, package cmd) are the ONLY callers — and
// neither lives under cmd/bootstrap. This test walks the cmd/bootstrap production
// source and fails if any file re-introduces a call to the hooks Store.CleanStale
// method or the shared runHookStaleCleanup helper.
//
// The `\bCleanStale\b` word-boundary is deliberate: the unrelated marker-sweep
// step Store method CleanStaleMarkers (which legitimately remains in the
// orchestrator as step 9) carries a trailing word char after "CleanStale", so
// the boundary excludes it — only the bare hooks CleanStale method matches.
//
// Only production (non-_test) files are scanned: the AC is about bootstrap
// STEPS (production code), and test files legitimately mention "CleanStale" in
// prose comments documenting the removal.
func TestHooksCleanStale_NoBootstrapStepIsAnAutomaticCaller(t *testing.T) {
	forbidden := regexp.MustCompile(`\bCleanStale\b|\brunHookStaleCleanup\b`)

	const bootstrapDir = "bootstrap"
	entries, err := os.ReadDir(bootstrapDir)
	if err != nil {
		t.Fatalf("read %s: %v", bootstrapDir, err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(bootstrapDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		scanned++
		if loc := forbidden.FindIndex(src); loc != nil {
			line := 1 + strings.Count(string(src[:loc[0]]), "\n")
			t.Errorf("%s:%d references the hooks CleanStale path; the daemon (maybeRunHookCleanup) "+
				"and `portal clean` are the only permitted automatic callers — a bootstrap step must not clean hooks",
				path, line)
		}
	}

	// Sanity: the directory must actually contain production source, else a
	// path/layout regression would make this guard silently vacuous.
	if scanned == 0 {
		t.Fatalf("no production .go files scanned under %q; guard is vacuous", bootstrapDir)
	}
}
