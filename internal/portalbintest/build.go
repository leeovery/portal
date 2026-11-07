// Package portalbintest provides test-only helpers for compiling the portal CLI
// binary and staging it on $PATH. It depends on nothing beyond stdlib and
// testing, so any test package can import it. Production code must not.
package portalbintest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ProjectRoot returns the nearest ancestor of the working directory containing
// go.mod, so a `go build` can be anchored regardless of which package's test
// binary is running.
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

// BuildPortalBinary compiles the portal CLI into dir/portal, returning an error
// carrying the failing build's output rather than fatalling, so a caller can
// choose between hard-fail and skip when `go` is unavailable.
func BuildPortalBinary(dir string) error {
	return buildPortalBinaryInto(dir)
}

// Build compiles the portal CLI into a fresh t.TempDir, failing the test if it
// can't, and returns the binary's absolute path.
func Build(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := BuildPortalBinary(dir); err != nil {
		t.Fatalf("build portal: %v", err)
	}
	return filepath.Join(dir, "portal")
}

// StagePortalBinary builds the portal CLI into a fresh t.TempDir and prepends
// that directory to $PATH, so a system-installed portal cannot shadow it. A build
// or lookup failure skips the test rather than failing it.
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

// -tags integration is load-bearing: it compiles the daemon-pgrep sandbox into
// the staged binary, so a subprocess bootstrap's orphan sweep honours the
// cross-process ownership registry and can never SIGKILL the developer's live
// daemon. The shipped release binary is built without the tag.
func buildPortalBinaryInto(dir string) error {
	binary := filepath.Join(dir, "portal")
	root, err := ProjectRoot()
	if err != nil {
		return fmt.Errorf("locate project root: %w", err)
	}
	cmd := exec.Command("go", "build", "-tags", "integration", "-o", binary, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}
