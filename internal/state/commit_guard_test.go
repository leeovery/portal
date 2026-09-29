package state_test

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestNoProductionCommitOutsideState(t *testing.T) {
	for _, finding := range scanForDirectCommit(t) {
		t.Errorf("%s reaches state.Commit, which takes no commit lock; commit through state.RunCommitCycle", finding)
	}
}

func TestCommitGuard_Rule(t *testing.T) {
	const callsCommit = `package fixture

import "github.com/leeovery/portal/internal/state"

func flush(dir string, idx state.Index) error {
	return state.Commit(dir, idx, true, nil)
}
`
	const aliasedCommit = `package fixture

import saved "github.com/leeovery/portal/internal/state"

var commit = saved.Commit
`
	const unrelatedCommit = `package fixture

import "github.com/leeovery/portal/internal/other/state"

func flush() { state.Commit() }
`
	const clean = "package fixture\n"

	for _, tc := range []struct {
		name         string
		files        map[string]string
		wantFindings []string
	}{
		{
			name:         "it fails for a production file outside internal/state calling state.Commit",
			files:        map[string]string{filepath.Join("cmd", "save.go"): callsCommit},
			wantFindings: []string{filepath.Join("cmd", "save.go")},
		},
		{
			name:         "it fails for a production file taking state.Commit as a value under an import alias",
			files:        map[string]string{filepath.Join("cmd", "doctor", "repair.go"): aliasedCommit},
			wantFindings: []string{filepath.Join("cmd", "doctor", "repair.go")},
		},
		{
			name: "it ignores state.Commit calls in test files outside internal/state",
			files: map[string]string{
				filepath.Join("cmd", "state_commit_now_test.go"): callsCommit,
				filepath.Join("cmd", "bootstrap", "run_test.go"): callsCommit,
				filepath.Join("cmd", "root.go"):                  clean,
			},
		},
		{
			name: "it ignores production files inside internal/state",
			files: map[string]string{
				filepath.Join("internal", "state", "commit_cycle.go"): callsCommit,
			},
		},
		{
			name: "it ignores a Commit selector on a package other than internal/state",
			files: map[string]string{
				filepath.Join("cmd", "other.go"): unrelatedCommit,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageCommitGuardTree(t, tc.files)

			findings := scanForDirectCommit(t, sourceguardtest.Rooted(root))

			if !slices.Equal(findings, tc.wantFindings) {
				t.Errorf("findings = %v, want %v", findings, tc.wantFindings)
			}
		})
	}
}

// A guard whose scan came back empty would report a clean tree forever.
func TestCommitGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanForDirectCommit(recorder, sourceguardtest.Rooted(t.TempDir()))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("scan of a tree holding no sources reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

func stageCommitGuardTree(t *testing.T, files map[string]string) string {
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

const statePkgPath = "github.com/leeovery/portal/internal/state"

// scanForDirectCommit reports every non-test file outside internal/state that
// names state.Commit, called or taken as a value, since either reaches a commit
// and its scrollback collection without the commit lock.
func scanForDirectCommit(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()

	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources, opts...)

	var findings []string
	for _, source := range sources {
		if filepath.Dir(source.Path) == filepath.Join("internal", "state") {
			continue
		}
		if namesStateCommit(source.File) {
			findings = append(findings, source.Path)
		}
	}
	return findings
}

func namesStateCommit(file *ast.File) bool {
	local, ok := stateImportName(file)
	if !ok {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		sel, isSel := n.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Commit" {
			return !found
		}
		if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == local {
			found = true
		}
		return !found
	})
	return found
}

func stateImportName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != statePkgPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true
		}
		return "state", true
	}
	return "", false
}
