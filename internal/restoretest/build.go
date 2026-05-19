// Untagged build helpers shared by default-lane and integration-lane
// callers. ProjectRoot + buildPortalBinaryInto previously lived in
// restoretest.go under `//go:build integration`; they were lifted here
// so default-tagged tests (notably
// internal/tmux/portal_saver_integration_test.go, which exercises the
// singleton-invariant acceptance under the default `go test ./...`
// lane) can reuse the same `go build` plumbing without re-inlining the
// project-root walk and exec.Command wiring.
//
// The integration-tagged wrappers in restoretest.go
// (BuildPortalBinaryDir, BuildPortalBinaryStable) delegate their build
// invocation to buildPortalBinaryInto so the underlying `go build .`
// command line lives in exactly one place.

package restoretest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ProjectRoot walks up from the current working directory until it finds
// a directory containing go.mod. Returns the absolute path of that
// directory. Used to anchor `go build` invocations regardless of the
// caller test binary's runtime CWD (cmd/, internal/restore/,
// internal/tmux/, etc.).
//
// Returns an error rather than fatalling so it can be reused by helpers
// that also return error (BuildPortalBinaryStable, BuildPortalBinary)
// without dragging *testing.T into pure plumbing.
//
// Exported because internal/restoretest/restoretest_test.go (external
// _test package) exercises this helper's contract directly, and because
// default-lane integration tests in internal/tmux/ also depend on it.
func ProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// BuildPortalBinary compiles the portal CLI into dir/portal using the
// repo-root-anchored `go build .` invocation. Returns a wrapped error
// on either project-root resolution failure or `go build` failure;
// stdout+stderr from the failing build are included in the returned
// error so the caller's diagnostic surface is unchanged from the
// previously-inlined helpers.
//
// Pure error-returning variant for callers that want to decide between
// hard-fail and clean-skip on build failure (e.g. the
// singleton-invariant test in internal/tmux/, which skips when `go`
// is not available rather than fatalling). The *testing.T-flavoured
// integration wrappers (BuildPortalBinaryDir, BuildPortalBinaryStable)
// call buildPortalBinaryInto directly so callers in restoretest.go do
// not pay the public-wrapper indirection.
func BuildPortalBinary(dir string) error {
	return buildPortalBinaryInto(dir)
}

// buildPortalBinaryInto compiles the portal CLI into dir/portal. Shared
// by BuildPortalBinaryDir, BuildPortalBinaryStable, and the public
// BuildPortalBinary wrapper so the underlying `go build` invocation
// lives in one place.
// StagePortalBinary builds the portal CLI into a fresh t.TempDir,
// prepends that directory to $PATH for the duration of the test, and
// asserts `portal` is resolvable via exec.LookPath. Returns the
// directory holding the built binary.
//
// Skip semantics mirror the inlined preambles previously duplicated
// across the default-lane real-tmux integration tests: a `go build`
// failure (no `go` on PATH, compile error) is a clean t.Skipf, as is
// a post-prepend exec.LookPath miss. Neither escalates to t.Fatal —
// the invariants these tests pin are structural, not "build works".
//
// PATH composition: binDir is prepended ahead of the inherited PATH so
// the freshly built binary cannot be shadowed by any system-installed
// `portal`. The t.Setenv contract restores the prior PATH on test
// exit.
func StagePortalBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	if err := BuildPortalBinary(binDir); err != nil {
		t.Skipf("portal binary build failed; skipping real-daemon integration test: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := exec.LookPath("portal"); err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}
	return binDir
}

func buildPortalBinaryInto(dir string) error {
	binary := filepath.Join(dir, "portal")
	root, err := ProjectRoot()
	if err != nil {
		return fmt.Errorf("locate project root: %w", err)
	}
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}
