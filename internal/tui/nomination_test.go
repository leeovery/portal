package tui

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
)

func testBuiltinPair(t *testing.T) theme.Nomination {
	t.Helper()
	return theme.AdaptivePair(testLightTheme(t), testDarkTheme(t))
}

func memberForSlot(slot theme.Slot) theme.Member {
	if slot == theme.SlotLight {
		return theme.MemberLight
	}
	return theme.MemberDark
}

func TestDeps_HasNoAppearanceField(t *testing.T) {
	depsType := reflect.TypeFor[Deps]()
	if _, found := depsType.FieldByName("Appearance"); found {
		t.Errorf("Deps still carries an Appearance field; the appearance injection is removed, not kept alongside Deps.Theme")
	}
	if _, found := depsType.FieldByName("Theme"); !found {
		t.Errorf("Deps carries no Theme field; the loaded nomination is what replaces the appearance injection")
	}

	for _, name := range exportedFuncsInPackage(t) {
		if name == "WithAppearance" {
			t.Errorf("WithAppearance is still declared; the option is removed rather than left as a second, dead injection path")
		}
	}
}

// An empty Theme renders silently through lipgloss.Color("")'s no-colour
// sentinel, so the render half is asserted too.
func TestNew_SeedsTheDarkBuiltinWhenNoNominationIsGiven(t *testing.T) {
	m := New(fakeLister{})

	if got, want := m.themeState.active, testDarkTheme(t); got != want {
		t.Errorf("activeTheme = %s, want the dark built-in %s", themeLabel(got), themeLabel(want))
	}
	assertActiveTheme(t, m, testDarkTheme(t).Canvas.Value)
}

func TestNomination_ConstantSkipsDetectionAndWait(t *testing.T) {
	light := testLightTheme(t)
	m := detectModel(t, theme.ConstantNomination(light))

	if !m.modeResolved() {
		t.Fatalf("a constant nomination left the first-paint gate open; want resolved at construction (no detection, no wait)")
	}
	m.armAppearanceDetection()
	if !m.modeResolved() {
		t.Errorf("arming re-opened a constant's gate; want it unarmable")
	}

	assertNoTimeoutTick(t, m)
	assertBackgroundQueryIssued(t, m)
	assertActiveTheme(t, m, light.Canvas.Value)
}

func TestNomination_AdaptiveArmsTheGate(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))

	if m.modeResolved() {
		t.Fatalf("an adaptive pair resolved at construction; want the detect-or-timeout gate open")
	}
	assertBlankFrame(t, m)
}

func TestNomination_GateSelectsMember(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
		want func(*testing.T) theme.Theme
	}{
		{"a dark OSC 11 reply selects the dark member", darkBg, testDarkTheme},
		{"a light OSC 11 reply selects the light member", lightBg, testLightTheme},
		{"the no-answer timeout selects the dark member", appearanceTimeoutMsg{}, testDarkTheme},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := detectModel(t, testBuiltinPair(t))

			updated, _ := m.Update(tc.msg)

			assertActiveTheme(t, updated.(Model), tc.want(t).Canvas.Value)
		})
	}
}

func TestGate_LateReplyCapturesBackgroundButNeverReThemes(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))

	timedOut, _ := m.Update(appearanceTimeoutMsg{})
	resolved := timedOut.(Model)
	assertActiveTheme(t, resolved, testDarkTheme(t).Canvas.Value)

	late, _ := resolved.Update(lightBg)
	after := late.(Model)

	if got := after.OriginalBackground(); got != "#e1e2e7" {
		t.Errorf("OriginalBackground() = %q after a late reply, want %q (the reply is still consumed)", got, "#e1e2e7")
	}
	if !after.themeState.reply.arrived {
		t.Errorf("reply.arrived = false after a late reply, want true (the arrival is retained for later classification)")
	}
	assertActiveTheme(t, after, testDarkTheme(t).Canvas.Value)
}

func TestGate_QueryIssuedRegardlessOfSettingShape(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*testing.T) Model
	}{
		{"constant", func(t *testing.T) Model {
			return Build(Deps{Lister: fakeLister{}, Theme: theme.ConstantNomination(testDarkTheme(t))})
		}},
		{"adaptive pair", func(t *testing.T) Model {
			return Build(Deps{Lister: fakeLister{}, Theme: testBuiltinPair(t)})
		}},
		{"NO_COLOR", func(t *testing.T) Model {
			return Build(Deps{Lister: fakeLister{}, Theme: testBuiltinPair(t), NoColor: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertBackgroundQueryIssued(t, tc.build(t))
		})
	}
}

func TestGate_ConstantRetainsReplyWithoutClassifying(t *testing.T) {
	dark := testDarkTheme(t)
	m := detectModel(t, theme.ConstantNomination(dark))

	updated, _ := m.Update(lightBg)
	after := updated.(Model)

	if got := after.OriginalBackground(); got != "#e1e2e7" {
		t.Errorf("OriginalBackground() = %q, want %q (the reply is retained for restore-on-exit)", got, "#e1e2e7")
	}
	if !after.themeState.reply.arrived {
		t.Errorf("reply.arrived = false, want true (a reply did arrive, whatever it said)")
	}
	if after.themeState.inForceMode() != theme.MemberDark {
		t.Errorf("inForceMode() = %v, want the standing dark fallback — a constant derives no light/dark answer from the reply", after.themeState.inForceMode())
	}
	assertActiveTheme(t, after, dark.Canvas.Value)
}

func TestNoColor_LoadsBothAndSelectsDark(t *testing.T) {
	m := Build(Deps{Lister: fakeLister{}, Theme: testBuiltinPair(t), NoColor: true})

	if !m.modeResolved() {
		t.Errorf("colourless model is unresolved; want the gate skipped (no canvas to select)")
	}
	if got, want := m.themeState.active, testDarkTheme(t); got != want {
		t.Errorf("activeTheme = %s, want the dark member %s (the standing no-answer fallback)", themeLabel(got), themeLabel(want))
	}
	if got, want := m.themeState.nomination.Select(theme.MemberLight), testLightTheme(t); got != want {
		t.Errorf("the light member is no longer held (Select(MemberLight) = %s, want %s); NO_COLOR must not skip loading either nomination", themeLabel(got), themeLabel(want))
	}
}

func TestConstruction_ReadsNoThemesDirectory(t *testing.T) {
	banned := map[string]string{
		"os.ReadDir":       "a directory read",
		"os.ReadFile":      "a file read",
		"os.Open":          "a file open",
		"os.OpenFile":      "a file open",
		"os.Stat":          "a filesystem probe",
		"os.Lstat":         "a filesystem probe",
		"os.Getenv":        "a config-environment read",
		"os.LookupEnv":     "a config-environment read",
		"filepath.Walk":    "a directory walk",
		"filepath.WalkDir": "a directory walk",
	}

	for file, calls := range packageCalls(t) {
		for _, call := range calls {
			if what, bad := banned[call]; bad {
				t.Errorf("%s calls %s (%s); TUI construction takes a LOADED nomination and must read nothing", file, call, what)
			}
			if strings.HasSuffix(call, ".Enumerate") {
				t.Errorf("%s calls %s; construction enumerates nothing — the themes directory is read only by the theme panel", file, call)
			}
		}
	}
}

// The request marker type is unexported, so the match is by type against a
// reference taken from tea.RequestBackgroundColor itself.
func assertBackgroundQueryIssued(t *testing.T, m Model) {
	t.Helper()
	wantType := reflect.TypeOf(tea.Cmd(tea.RequestBackgroundColor)())
	for _, msg := range initCmds(t, m.Init()) {
		if reflect.TypeOf(msg) == wantType {
			return
		}
	}
	t.Errorf("Init issued no OSC 11 background query (no %v produced); restore-on-exit and the adaptive conversion both need the reply", wantType)
}

func exportedFuncsInPackage(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, file := range parsePackageFilesByName(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.IsExported() {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names
}

func packageCalls(t *testing.T) map[string][]string {
	t.Helper()
	calls := map[string][]string{}
	for name, file := range parsePackageFilesByName(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			calls[name] = append(calls[name], ident.Name+"."+sel.Sel.Name)
			return true
		})
	}
	return calls
}

// go test runs in the package's source directory, so "." is the package dir.
func parsePackageFilesByName(t *testing.T) map[string]*ast.File {
	t.Helper()

	return filesByName(sourceguardtest.ParsePackageSources(t, ".", false))
}

// filesByName keys an already-parsed set by base name, so a guard that needs
// both the set and the lookup parses the package once.
func filesByName(sources []sourceguardtest.ParsedSource) map[string]*ast.File {
	files := make(map[string]*ast.File, len(sources))
	for _, source := range sources {
		files[filepath.Base(source.Path)] = source.File
	}
	return files
}
