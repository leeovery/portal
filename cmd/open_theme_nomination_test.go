package cmd

import (
	"go/ast"
	"go/token"
	"io"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/spf13/cobra"
)

// Three files are exempt in full because each is a separate verb no `portal
// open` invocation reaches. For the two whose loader is silent, the runtime half
// does not back that exemption up: doctor hands its loader log.Discard(), so a
// doctor-side helper called from the exec path would read the poisoned directory
// and still write no record.
func TestOpenExecPath_DoesNoThemeWork(t *testing.T) {
	t.Run("no theme call site sits outside TUI construction", func(t *testing.T) {
		allowed := map[string]bool{
			"openTUI":          true,
			"themeResolution":  true,
			"buildThemeLoader": true,
			"newThemeLoader":   true,
			// A pure prefs→theme key conversion: it reads no directory and
			// loads nothing, so it does no theme work to keep off the exec path.
			"themeRawKeys": true,
			// newThemeSource is deliberately absent: these names are matched in
			// every file, so permitting it would licence a construction-time
			// sweep anywhere. The `local` map below tracks it instead.
		}

		exemptFiles := map[string]bool{"theme.go": true, "doctor_theme.go": true, "state_resume_draw.go": true}

		for file, callers := range themeCallSites(t) {
			if exemptFiles[file] {
				continue
			}
			for fn, call := range callers {
				if !allowed[fn] {
					t.Errorf("%s: %s calls %s — theme work belongs to TUI construction; the exec path constructs no TUI and must stay free of it", file, fn, call)
				}
			}
		}
	})

	t.Run("an exec-path open emits no theme record", func(t *testing.T) {
		// An existing but unreadable themes directory earns a `theme: directory
		// unusable` WARN, where an absent one is silent.
		_ = themetest.DenyDir(t, useThemesDir(t))
		// A drop-in slug: a built-in would resolve out of the embedded set and
		// never touch the poison.
		setPrefsFile(t, `{"theme":"a-drop-in"}`)

		loud := logtest.Install(t)
		themeNominationForTest(t)
		if len(themeEvents(t, loud)) == 0 {
			t.Fatal("the construction-time resolution emitted no theme record against the poisoned directory; the zero-record assertion below would be vacuous")
		}

		sink := logtest.Install(t)

		if got := execOpenSession(t, "api-x7Kd9a"); got != "api-x7Kd9a" {
			t.Fatalf("open attached %q, want the session it resolved — the exec path must have run", got)
		}

		assertThemeEvents(t, sink)
	})
}

// Every tmux-touching seam is injected, so the body runs its production
// resolution without reaching a server.
func execOpenSession(t *testing.T, name string) string {
	t.Helper()

	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})
	withOpenDeps(t, OpenDeps{
		SessionLister: &testSessionLister{names: []string{name}},
		AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
		Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator:  &testDirValidator{existing: map[string]bool{}},
	})

	var attached string
	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, target string) error {
		attached = target
		return nil
	})

	resetRootCmd()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"open", name})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("portal open %s: %v", name, err)
	}
	return attached
}

// The component is legitimately emitted from more than one package, so a second
// binding inside cmd would be invisible at review — every call site would still
// look correct in isolation.
func TestThemeComponent_BoundOnceInCmd(t *testing.T) {
	bindings := componentBindings(t, "theme")

	if len(bindings) != 1 {
		t.Fatalf(`cmd binds the "theme" log component %d times via %v, want exactly 1 (a single package-level themeLogger)`, len(bindings), bindings)
	}
	if bindings[0] != "themeLogger" {
		t.Errorf(`the "theme" component is bound to %q, want the package-level var "themeLogger"`, bindings[0])
	}
}

func assertConstant(t *testing.T, n theme.Nomination, want theme.Theme) {
	t.Helper()
	if !n.IsConstant() {
		t.Fatalf("nomination is not constant; a pinned appearance must become a pinned THEME, with detection still off")
	}
	if got := n.Constant(); got != want {
		t.Errorf("constant = %s, want %s", canvasOf(got), canvasOf(want))
	}
}

// A whole Theme through %+v is a {name value} pair per token of noise.
func canvasOf(th theme.Theme) string {
	if th.Canvas.Value == "" {
		return "zero-theme"
	}
	return "theme(canvas " + th.Canvas.Value + ")"
}

func themeCallSites(t *testing.T) map[string]map[string]string {
	t.Helper()
	local := map[string]bool{
		"themeResolution":  true,
		"buildThemeLoader": true,
		"newThemeLoader":   true,
		"newThemeSource":   true,
		// Entry points into doctor's scan, tracked because their file is exempt in
		// full — otherwise every helper it declares would be invisible here.
		//
		// doctor's own caller escapes the scan only because it sits in doctorCmd's
		// composite-literal RunE, which is not an *ast.FuncDecl. Extracting it into
		// a named function trips this guard: keep the call in the literal rather
		// than widening `allowed`, whose names match in every file.
		"collectThemeAdvisories": true,
		"themeAdvisoryUnion":     true,
	}
	sites := map[string]map[string]string{}

	for name, file := range parsePackageFilesByName(t) {
		sourceguardtest.ForEachFuncCall(file, func(funcName string, call *ast.CallExpr) bool {
			switch target := call.Fun.(type) {
			case *ast.SelectorExpr:
				pkg, ok := target.X.(*ast.Ident)
				if !ok || pkg.Name != "theme" {
					return true
				}
				record(sites, name, funcName, "theme."+target.Sel.Name)
			case *ast.Ident:
				if local[target.Name] {
					record(sites, name, funcName, target.Name)
				}
			}
			return true
		})
	}
	return sites
}

func record(sites map[string]map[string]string, file, fn, call string) {
	if sites[file] == nil {
		sites[file] = map[string]string{}
	}
	sites[file][fn] = call
}

// Returns the var name for the sanctioned package-level shape, and
// "file:function" for a call inside a function body.
func componentBindings(t *testing.T, component string) []string {
	t.Helper()
	var bound []string
	for _, file := range parsePackageFilesByName(t) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, expr := range value.Values {
					if isLogForCall(expr, component) && i < len(value.Names) {
						bound = append(bound, value.Names[i].Name)
					}
				}
			}
		}
	}
	return append(bound, logForCalls(t, component)...)
}

func isLogForCall(expr ast.Expr, component string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "For" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "log" {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"`+component+`"`
}

func logForCalls(t *testing.T, component string) []string {
	t.Helper()
	var found []string
	for name, file := range parsePackageFilesByName(t) {
		sourceguardtest.ForEachFuncCall(file, func(funcName string, call *ast.CallExpr) bool {
			if isLogForCall(call, component) {
				found = append(found, name+":"+funcName)
			}
			return true
		})
	}
	return found
}

// go test runs in the package's source directory, so the "." enumeration
// resolves wherever the suite was invoked from.
func parsePackageFilesByName(t *testing.T) map[string]*ast.File {
	t.Helper()
	sources := sourceguardtest.ParsePackageSources(t, ".", false)
	files := make(map[string]*ast.File, len(sources))
	for _, source := range sources {
		files[filepath.Base(source.Path)] = source.File
	}
	return files
}
