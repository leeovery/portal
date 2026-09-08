package cmd

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/xdg"
)

// fileIdentity is one expected config-file identity: the name it is declared
// under in internal/xdg beside the three values that declaration must carry.
type fileIdentity struct {
	name         string
	id           xdg.ConfigFileID
	envVar       string
	filename     string
	logComponent string
}

// dirIdentity is one expected config-directory identity. A directory carries no
// log component — nothing about it is migrated or announced.
type dirIdentity struct {
	name    string
	id      xdg.ConfigDirID
	envVar  string
	dirname string
}

// wantFileIdentities pins every config file Portal reads: the variable the user
// points at it with, the name it takes under the config base, and the log
// component that speaks for it. A typo in any of these ships a store that reads
// a file nothing writes — one that simply appears empty to its owner.
func wantFileIdentities() []fileIdentity {
	return []fileIdentity{
		{name: "ProjectsFile", id: xdg.ProjectsFile, envVar: "PORTAL_PROJECTS_FILE", filename: "projects.json", logComponent: "projects"},
		{name: "AliasesFile", id: xdg.AliasesFile, envVar: "PORTAL_ALIASES_FILE", filename: "aliases", logComponent: "aliases"},
		{name: "HooksFile", id: xdg.HooksFile, envVar: "PORTAL_HOOKS_FILE", filename: "hooks.json", logComponent: "hooks"},
		{name: "PrefsFile", id: xdg.PrefsFile, envVar: "PORTAL_PREFS_FILE", filename: "prefs.json", logComponent: ""},
		{name: "TerminalsFile", id: xdg.TerminalsFile, envVar: "PORTAL_TERMINALS_FILE", filename: "terminals.json", logComponent: ""},
	}
}

// wantDirIdentities pins Portal's directories under the config base.
func wantDirIdentities() []dirIdentity {
	return []dirIdentity{
		{name: "StateDir", id: xdg.StateDir, envVar: "PORTAL_STATE_DIR", dirname: "state"},
		{name: "ThemesDir", id: xdg.ThemesDir, envVar: "PORTAL_THEMES_DIR", dirname: "themes"},
	}
}

// TestConfigFileIdentity pins what a single declaration of a config file's
// identity buys: the production route, the shared rule and the test seeder all
// name the same file because they all read the same pair, so renaming either
// half cannot move one route without moving the others.
func TestConfigFileIdentity(t *testing.T) {
	t.Run("it resolves hooks.json from the shared file identity", func(t *testing.T) {
		base := t.TempDir()
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", base)
		t.Setenv("PORTAL_HOOKS_FILE", "")
		env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + base}

		shared, err := xdg.ConfigFilePath(xdg.EnvSlice(env), xdg.HooksFile)
		if err != nil {
			t.Fatalf("shared rule: %v", err)
		}

		production, err := hooksFilePath()
		if err != nil {
			t.Fatalf("production route: %v", err)
		}
		seeder := hookstest.ResolveHooksFilePathFromEnv(t, env)

		want := filepath.Join(base, "portal", "hooks.json")
		for _, got := range []struct {
			route string
			path  string
		}{
			{"the shared identity", shared.Path},
			{"the production route", production},
			{"the seeder", seeder},
		} {
			if got.path != want {
				t.Errorf("%s resolved %q, want %q", got.route, got.path, want)
			}
		}
	})

	t.Run("it pins every config file identity's env var, filename and log component", func(t *testing.T) {
		for _, want := range wantFileIdentities() {
			t.Run(want.name, func(t *testing.T) {
				if got := want.id.EnvVar; got != want.envVar {
					t.Errorf("%s.EnvVar = %q, want %q", want.name, got, want.envVar)
				}
				if got := want.id.Filename; got != want.filename {
					t.Errorf("%s.Filename = %q, want %q", want.name, got, want.filename)
				}
				if got := want.id.LogComponent; got != want.logComponent {
					t.Errorf("%s.LogComponent = %q, want %q", want.name, got, want.logComponent)
				}
			})
		}
	})

	t.Run("it pins the deliberately empty log components rather than skipping them", func(t *testing.T) {
		var empty []string
		for _, want := range wantFileIdentities() {
			if want.id.LogComponent == "" {
				empty = append(empty, want.name)
			}
		}
		slices.Sort(empty)

		wantEmpty := []string{"PrefsFile", "TerminalsFile"}
		if !slices.Equal(empty, wantEmpty) {
			t.Errorf("identities with the empty log component = %v, want %v — a file outside the closed log vocabulary is a deliberate choice, so gaining or losing one is a visible edit", empty, wantEmpty)
		}
	})
}

// TestConfigDirIdentity pins Portal's config-directory identities. A directory
// takes its location by the same rule as a config file, and its `_DIR` variable
// and name are as easy to mistype and as silent when mistyped.
func TestConfigDirIdentity(t *testing.T) {
	t.Run("it pins both config directory identities' env var and directory name", func(t *testing.T) {
		for _, want := range wantDirIdentities() {
			t.Run(want.name, func(t *testing.T) {
				if got := want.id.EnvVar; got != want.envVar {
					t.Errorf("%s.EnvVar = %q, want %q", want.name, got, want.envVar)
				}
				if got := want.id.Dirname; got != want.dirname {
					t.Errorf("%s.Dirname = %q, want %q", want.name, got, want.dirname)
				}
			})
		}
	})
}

// The type names internal/xdg declares its identities as, which is how the
// completeness guard tells one kind of declaration from the other.
const (
	fileIDType = "ConfigFileID"
	dirIDType  = "ConfigDirID"
)

// TestConfigIdentityCompleteness derives the identity set from the declarations
// themselves, so the pinning above cannot be outgrown: an identity added to
// internal/xdg without a row here is unpinned, and unpinned is how a typo ships.
func TestConfigIdentityCompleteness(t *testing.T) {
	t.Run("it fails when an identity is declared with no table row", func(t *testing.T) {
		declared := declaredIdentities(t, filepath.Join(sourceguardtest.ProjectRoot(t), "internal", "xdg"))

		rowed := map[string][]string{}
		for _, want := range wantFileIdentities() {
			rowed[fileIDType] = append(rowed[fileIDType], want.name)
		}
		for _, want := range wantDirIdentities() {
			rowed[dirIDType] = append(rowed[dirIDType], want.name)
		}

		for _, kind := range []string{fileIDType, dirIDType} {
			assertSameNames(t, kind, declared[kind], rowed[kind])
		}
	})
}

// declaredIdentities returns the package-level variables dir's non-test sources
// initialise with a struct composite literal, grouped by the name of the struct
// type each holds.
func declaredIdentities(t *testing.T, dir string) map[string][]string {
	t.Helper()

	declared := map[string][]string{}
	for _, src := range sourceguardtest.ParsePackageSources(t, dir, false) {
		for _, decl := range src.File.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					kind, ok := compositeLitTypeName(value, i)
					if !ok {
						continue
					}
					declared[kind] = append(declared[kind], name.Name)
				}
			}
		}
	}
	return declared
}

// compositeLitTypeName reports the name of the struct type the i-th value of a
// var spec is a composite literal of.
func compositeLitTypeName(spec *ast.ValueSpec, i int) (string, bool) {
	if i >= len(spec.Values) {
		return "", false
	}
	lit, ok := spec.Values[i].(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	ident, ok := lit.Type.(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// assertSameNames reports each declared identity of a kind that carries no row,
// and each row naming an identity no longer declared.
func assertSameNames(t *testing.T, kind string, declared, rowed []string) {
	t.Helper()

	have := map[string]bool{}
	for _, name := range rowed {
		have[name] = true
	}
	for _, name := range declared {
		if !have[name] {
			t.Errorf("xdg.%s is declared as a %s and pinned by no table row — add one, or a typo in it ships silently", name, kind)
		}
		delete(have, name)
	}

	for _, name := range slices.Sorted(maps.Keys(have)) {
		t.Errorf("table row names xdg.%s as a %s, which internal/xdg no longer declares", name, kind)
	}
}
