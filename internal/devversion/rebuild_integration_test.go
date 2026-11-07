//go:build integration

package devversion_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The proof the version exists for, on a real toolchain: rebuilding a binary
// with different content, as an edit would, to the same path changes the dev
// version it reports.
func TestString_ARebuildChangesIt(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "printversion")

	first := buildAndRun(t, binary, "first")
	second := buildAndRun(t, binary, "second")

	if first == second {
		t.Errorf("both builds report %q, want the rebuild to report its own", first)
	}
	for _, v := range []string{first, second} {
		if !strings.HasPrefix(v, "dev-") {
			t.Errorf("version = %q, want a dev version", v)
		}
	}
}

// buildAndRun builds the printversion helper to binary with build injected,
// runs it, and returns the dev version it prints.
func buildAndRun(t *testing.T, binary, build string) string {
	t.Helper()
	cmd := exec.Command("go", "build", "-ldflags=-X=main.build="+build, "-o", binary, "./testdata/printversion")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run %s: %v", binary, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || lines[1] != build {
		t.Fatalf("printversion printed %q, want its version then %q: the build did not land", out, build)
	}
	return lines[0]
}
