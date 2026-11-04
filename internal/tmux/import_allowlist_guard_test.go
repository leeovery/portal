package tmux_test

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const tmuxPkgPath = "github.com/leeovery/portal/internal/tmux"

// tmuxImportAllowList is every production source still on tmux, each with the
// phase of docs/design/zmx-backend.md that moves it off, so a phase shrinks the
// list by deleting its lines. In cmd, which mixes zmx and tmux code, an entry
// names a file; elsewhere it names a package directory.
var tmuxImportAllowList = map[string]int{
	"cmd/state_hydrate.go":          3,
	"cmd/state_resume_altscreen.go": 3,
	"cmd/state_resume_recover.go":   3,
	"cmd/state_resume_report.go":    3,
	"cmd/state_resume_wait.go":      3,
	"cmd/state_signal_hydrate.go":   3,
	"internal/hooksweep":            3,
}

// tmuxTestOnlyPackages serve tests alone, so they are outside the guard; they
// go with internal/tmux in phase 5.
var tmuxTestOnlyPackages = []string{
	"internal/tmuxtest",
	"internal/transienttest",
}

// zmxOnlyPackages are the packages the phase 1 surfaces and the phase 2
// attach run on. They are never allow-listed: nothing tmux-shaped survives
// above the driver.
var zmxOnlyPackages = []string{
	"internal/attachkeys",
	"internal/backend",
	"internal/proctable",
	"internal/resolver",
	"internal/runner",
	"internal/runnerreg",
	"internal/runnertoken",
	"internal/session",
	"internal/sessionindex",
	"internal/spawn",
	"internal/tui",
}

func TestNoProductionTmuxImportOutsideTheAllowList(t *testing.T) {
	unlisted, stale := scanTmuxImports(t, tmuxImportAllowList)
	for _, source := range unlisted {
		t.Errorf("%s imports internal/tmux; nothing tmux-shaped survives above the zmx driver", source)
	}
	for _, entry := range stale {
		t.Errorf("allow-listed %s no longer imports internal/tmux; delete its line (phase %d)", entry, tmuxImportAllowList[entry])
	}
}

func TestTmuxImportAllowList_NamesFilesInCmdAndPackagesElsewhere(t *testing.T) {
	for entry := range tmuxImportAllowList {
		isFile := strings.HasSuffix(entry, ".go")
		if entry == "cmd" || isFile && path.Dir(entry) != "cmd" {
			t.Errorf("allow-list entry %s: name a file in cmd and a package elsewhere", entry)
		}
		for _, pkg := range zmxOnlyPackages {
			if entry == pkg || strings.HasPrefix(entry, pkg+"/") {
				t.Errorf("allow-list entry %s is in %s, which runs on zmx alone", entry, pkg)
			}
		}
	}
}

// A zmx-only entry that names no package would guard nothing.
func TestZmxOnlyPackages_NameProductionPackages(t *testing.T) {
	root := sourceguardtest.ProjectRoot(t)
	for _, pkg := range zmxOnlyPackages {
		if files, err := sourceguardtest.PackageGoFiles(filepath.Join(root, filepath.FromSlash(pkg)), false); err != nil || len(files) == 0 {
			t.Errorf("zmx-only entry %s holds no production source (%v)", pkg, err)
		}
	}
}

func TestTmuxImportGuard_Rule(t *testing.T) {
	const importsTmux = "package fixture\n\nimport \"" + tmuxPkgPath + "\"\n\nvar _ tmux.Target\n"
	const aliasesTmux = "package fixture\n\nimport _ \"" + tmuxPkgPath + "\"\n"
	const clean = "package fixture\n"
	allowed := map[string]int{"cmd/listed.go": 2, "internal/listed": 3}

	for _, tc := range []struct {
		name         string
		files        map[string]string
		wantUnlisted []string
		wantStale    []string
	}{
		{
			name: "it reports an unlisted production import, aliased or not",
			files: map[string]string{
				"cmd/listed.go":          importsTmux,
				"internal/listed/a.go":   importsTmux,
				"cmd/picker.go":          importsTmux,
				"internal/tui/model.go":  aliasesTmux,
				"internal/tui/render.go": clean,
				"internal/listed/b.go":   clean,
				"cmd/list.go":            clean,
			},
			wantUnlisted: []string{"cmd/picker.go", "internal/tui/model.go"},
		},
		{
			name: "it lets a package entry cover each of its files but a file entry cover only itself",
			files: map[string]string{
				"cmd/listed.go":        importsTmux,
				"cmd/listed_more.go":   importsTmux,
				"internal/listed/a.go": importsTmux,
				"internal/listed/b.go": importsTmux,
			},
			wantUnlisted: []string{"cmd/listed_more.go"},
		},
		{
			name: "it ignores test files and the test-only packages",
			files: map[string]string{
				"cmd/listed.go":               importsTmux,
				"internal/listed/a.go":        importsTmux,
				"cmd/picker_test.go":          importsTmux,
				"internal/tmuxtest/socket.go": importsTmux,
				"internal/transienttest/t.go": importsTmux,
				"internal/tui/model_test.go":  aliasesTmux,
				"internal/backend/backend.go": clean,
			},
		},
		{
			name: "it reports an entry that no longer imports tmux",
			files: map[string]string{
				"cmd/listed.go":        clean,
				"internal/listed/a.go": clean,
			},
			wantStale: []string{"cmd/listed.go", "internal/listed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageTmuxImportTree(t, tc.files)

			unlisted, stale := scanTmuxImports(t, allowed, sourceguardtest.Rooted(root))

			if !slices.Equal(unlisted, tc.wantUnlisted) {
				t.Errorf("unlisted = %v, want %v", unlisted, tc.wantUnlisted)
			}
			if !slices.Equal(stale, tc.wantStale) {
				t.Errorf("stale = %v, want %v", stale, tc.wantStale)
			}
		})
	}
}

func TestTmuxImportGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanTmuxImports(recorder, tmuxImportAllowList, sourceguardtest.Rooted(t.TempDir()))
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

// scanTmuxImports returns, sorted, each production source importing
// internal/tmux that allowed does not cover, and each allowed entry that covers
// no such source.
func scanTmuxImports(t harnesstest.TestingT, allowed map[string]int, opts ...sourceguardtest.ScanOption) (unlisted, stale []string) {
	t.Helper()

	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources, opts...)

	covered := map[string]bool{}
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		pkg := path.Dir(file)
		if slices.Contains(tmuxTestOnlyPackages, pkg) || !importsTmuxPackage(source) {
			continue
		}
		switch {
		case hasEntry(allowed, file):
			covered[file] = true
		case hasEntry(allowed, pkg):
			covered[pkg] = true
		default:
			unlisted = append(unlisted, file)
		}
	}
	for entry := range allowed {
		if !covered[entry] {
			stale = append(stale, entry)
		}
	}
	slices.Sort(unlisted)
	slices.Sort(stale)
	return unlisted, stale
}

func hasEntry(allowed map[string]int, entry string) bool {
	_, ok := allowed[entry]
	return ok
}

func importsTmuxPackage(source sourceguardtest.ParsedSource) bool {
	return slices.ContainsFunc(source.File.Imports, func(spec *ast.ImportSpec) bool {
		p, err := strconv.Unquote(spec.Path.Value)
		return err == nil && p == tmuxPkgPath
	})
}
