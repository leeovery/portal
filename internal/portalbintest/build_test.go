package portalbintest_test

// Untagged tests for the build/stage helpers in
// internal/portalbintest/build.go. These mirror the default-lane usage
// pattern: callers in internal/tmux/ and cmd/ that depend on a real
// `portal` binary and a PATH-prepended bin directory but compile under
// `go test ./...` without the integration tag.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// TestProjectRoot asserts ProjectRoot walks up from the test's runtime CWD
// until it finds the directory containing the repository's go.mod. The
// integration test packages (cmd/bootstrap, internal/restore, cmd) all
// rely on this to compile the portal CLI from the repo root.
func TestProjectRoot(t *testing.T) {
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected go.mod under %s: %v", root, err)
	}
	// Sanity: the located module should be the portal module. We read
	// the first line of go.mod and assert the module path matches; a
	// false positive (e.g. a stray go.mod in a parent dir) would
	// otherwise pass the os.Stat check above.
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	want := "module github.com/leeovery/portal"
	if !strings.Contains(string(data), want) {
		t.Errorf("go.mod at %s does not declare %q; got:\n%s", root, want, data)
	}
}

// TestStagePortalBinary asserts the helper composes
// BuildPortalBinary + t.Setenv("PATH", ...) + exec.LookPath("portal")
// into a single call. The returned directory must contain the freshly
// built binary, PATH must have binDir prepended ahead of the inherited
// PATH, and `portal` must be resolvable via exec.LookPath after the
// helper returns.
func TestStagePortalBinary(t *testing.T) {
	priorPATH := os.Getenv("PATH")

	binDir := portalbintest.StagePortalBinary(t)

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
