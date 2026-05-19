package restoretest_test

// Untagged tests for the shared build/stage helpers in
// internal/restoretest/build.go. These mirror the default-lane usage
// pattern: callers in internal/tmux/ and cmd/ that depend on a real
// `portal` binary and a PATH-prepended bin directory but compile under
// `go test ./...` without the integration tag.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestStagePortalBinary asserts the helper composes
// BuildPortalBinary + t.Setenv("PATH", ...) + exec.LookPath("portal")
// into a single call. The returned directory must contain the freshly
// built binary, PATH must have binDir prepended ahead of the inherited
// PATH, and `portal` must be resolvable via exec.LookPath after the
// helper returns.
func TestStagePortalBinary(t *testing.T) {
	priorPATH := os.Getenv("PATH")

	binDir := restoretest.StagePortalBinary(t)

	if binDir == "" {
		t.Fatalf("StagePortalBinary returned empty binDir")
	}

	// Binary must exist at binDir/portal.
	binary := filepath.Join(binDir, "portal")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("expected portal binary at %s: %v", binary, err)
	}

	// PATH must start with binDir followed by the OS list separator and
	// the prior PATH. This pins the prepend order (binDir first) so a
	// system-installed `portal` cannot shadow the freshly built one.
	gotPATH := os.Getenv("PATH")
	wantPrefix := binDir + string(os.PathListSeparator)
	if !strings.HasPrefix(gotPATH, wantPrefix) {
		t.Fatalf("PATH not prepended with binDir; got %q, want prefix %q", gotPATH, wantPrefix)
	}
	if !strings.HasSuffix(gotPATH, priorPATH) {
		t.Fatalf("PATH does not retain prior PATH as suffix; got %q, want suffix %q", gotPATH, priorPATH)
	}

	// portal must be resolvable on PATH after the helper returns.
	resolved, err := exec.LookPath("portal")
	if err != nil {
		t.Fatalf("exec.LookPath(\"portal\") after StagePortalBinary: %v", err)
	}
	// The resolved path should live under binDir (not some pre-existing
	// system portal). Use EvalSymlinks on both sides so a symlinked
	// $TMPDIR (macOS /var → /private/var) does not produce a false
	// negative.
	resolvedReal, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", resolved, err)
	}
	binDirReal, err := filepath.EvalSymlinks(binDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", binDir, err)
	}
	if filepath.Dir(resolvedReal) != binDirReal {
		t.Fatalf("portal resolved outside staged binDir; got %s, want under %s", resolvedReal, binDirReal)
	}
}
