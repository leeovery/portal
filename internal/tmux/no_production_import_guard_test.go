package tmux_test

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const tmuxPkgPath = "github.com/leeovery/portal/internal/tmux"

// tmuxTestOnlyPackage serves tests alone, so it is outside the guard; it goes
// with internal/tmux in phase 5.
const tmuxTestOnlyPackage = "internal/tmuxtest"

func TestNoProductionSourceImportsTmux(t *testing.T) {
	for _, source := range scanTmuxImports(t) {
		t.Errorf("%s imports internal/tmux; Portal runs on zmx, and nothing tmux-shaped survives above the driver", source)
	}
}

func TestTmuxImportGuard_Rule(t *testing.T) {
	const importsTmux = "package fixture\n\nimport \"" + tmuxPkgPath + "\"\n\nvar _ tmux.Target\n"
	const aliasesTmux = "package fixture\n\nimport _ \"" + tmuxPkgPath + "\"\n"
	const clean = "package fixture\n"

	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "it reports every production import, aliased or not",
			files: map[string]string{
				"cmd/picker.go":          importsTmux,
				"internal/tui/model.go":  aliasesTmux,
				"internal/tui/render.go": clean,
				"cmd/list.go":            clean,
			},
			want: []string{"cmd/picker.go", "internal/tui/model.go"},
		},
		{
			name: "it ignores test files and the test-only package",
			files: map[string]string{
				"cmd/picker_test.go":          importsTmux,
				"internal/tmuxtest/socket.go": importsTmux,
				"internal/tui/model_test.go":  aliasesTmux,
				"internal/backend/backend.go": clean,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageTmuxImportTree(t, tc.files)

			got := scanTmuxImports(t, sourceguardtest.Rooted(root))

			if !slices.Equal(got, tc.want) {
				t.Errorf("importers = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTmuxImportGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanTmuxImports(recorder, sourceguardtest.Rooted(t.TempDir()))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("scan of a tree holding no sources reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

func stageTmuxImportTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// scanTmuxImports returns, sorted, each production source outside the
// test-only package that imports internal/tmux.
func scanTmuxImports(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()

	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources, opts...)

	var importers []string
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		if path.Dir(file) == tmuxTestOnlyPackage || !importsTmuxPackage(source) {
			continue
		}
		importers = append(importers, file)
	}
	slices.Sort(importers)
	return importers
}

func importsTmuxPackage(source sourceguardtest.ParsedSource) bool {
	return slices.ContainsFunc(source.File.Imports, func(spec *ast.ImportSpec) bool {
		p, err := strconv.Unquote(spec.Path.Value)
		return err == nil && p == tmuxPkgPath
	})
}
