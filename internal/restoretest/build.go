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
