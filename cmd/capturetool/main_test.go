package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func TestFlags_AreFixtureAndThemeOnly(t *testing.T) {
	flags := registeredStringFlags(t)

	names := make([]string, 0, len(flags))
	for name := range flags {
		names = append(names, name)
	}
	slices.Sort(names)

	if want := []string{"fixture", "theme"}; !slices.Equal(names, want) {
		t.Errorf("registered flags = %v, want %v — a theme IS the mode, so there is no appearance left to pin", names, want)
	}
	if got, want := flags["theme"], "defaultThemeSlug"; got != want {
		t.Errorf("--theme default = %s, want the %s constant", got, want)
	}
}

func registeredStringFlags(t *testing.T) map[string]string {
	t.Helper()

	file := sourceguardtest.PackageSource(t, ".", "main.go").File

	flags := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "String" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "flag" {
			return true
		}
		name, ok := call.Args[0].(*ast.BasicLit)
		if !ok || name.Kind != token.STRING {
			return true
		}
		flags[strings.Trim(name.Value, `"`)] = types.ExprString(call.Args[1])
		return true
	})
	return flags
}

func TestResolveTheme_DefaultsToTheShippedDarkBuiltin(t *testing.T) {
	if defaultThemeSlug != theme.DefaultDarkSlug {
		t.Fatalf("defaultThemeSlug = %q, want the shipped dark default %q", defaultThemeSlug, theme.DefaultDarkSlug)
	}
	if got, want := declaredConstSource(t, "defaultThemeSlug"), "theme.DefaultDarkSlug"; got != want {
		t.Errorf("defaultThemeSlug is declared as %s, want %s — the capture default follows the shipped dark default rather than restating it", got, want)
	}

	got, err := resolveTheme(theme.NewSilentLoader(), defaultThemeSlug, io.Discard)
	if err != nil {
		t.Fatalf("resolveTheme(%s): %v", defaultThemeSlug, err)
	}

	want, rejection, found := theme.NewSilentLoader().LoadBuiltin(theme.DefaultDarkSlug)
	if !found || rejection != nil {
		t.Fatalf("LoadBuiltin(%s) found=%v rejection=%v", theme.DefaultDarkSlug, found, rejection)
	}
	if got != want.Theme {
		t.Errorf("resolveTheme(%s) did not return the embedded %s palette", defaultThemeSlug, theme.DefaultDarkSlug)
	}
}

func declaredConstSource(t *testing.T, name string) string {
	t.Helper()

	file := sourceguardtest.PackageSource(t, ".", "main.go").File

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 || value.Names[0].Name != name {
				continue
			}
			return types.ExprString(value.Values[0])
		}
	}

	t.Fatalf("main.go declares no const %s", name)
	return ""
}

func TestThemeArg_SlugVersusPath(t *testing.T) {
	tests := []struct {
		name   string
		arg    string
		isPath bool
	}{
		{name: "a bare slug", arg: "nord"},
		{name: "a bare .theme filename", arg: "nord.theme", isPath: true},
		{name: "a relative .theme path", arg: "./nord.theme", isPath: true},
		{name: "an absolute .theme path", arg: "/abs/nord.theme", isPath: true},
		{name: "a nested path with no extension", arg: "sub/dir/x", isPath: true},
		{name: "a relative path with an unexpected extension", arg: "./mytheme.txt", isPath: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isThemePath(tt.arg); got != tt.isPath {
				t.Errorf("isThemePath(%q) = %v, want %v", tt.arg, got, tt.isPath)
			}
		})
	}
}

func TestResolveTheme_UnknownSlugIsAnError(t *testing.T) {
	got, err := resolveTheme(theme.NewSilentLoader(), "not-a-theme", io.Discard)
	if err == nil {
		t.Fatal("resolveTheme(not-a-theme) returned nil error, want an error")
	}
	if !strings.Contains(err.Error(), "not-a-theme") {
		t.Errorf("error %q does not name the slug that was asked for", err.Error())
	}
	if got != (theme.Theme{}) {
		t.Error("resolveTheme returned a palette alongside its error")
	}
}

func TestResolveTheme_InvalidBuiltinIsAnErrorNotAFallback(t *testing.T) {
	rejection := &theme.Rejection{Reason: theme.ReasonMissingTokens, Detail: "missing canvas, border"}

	got, err := resolveTheme(rejectingLoader{rejection: rejection}, "tokyo-night", io.Discard)
	if err == nil {
		t.Fatal("resolveTheme returned nil error for a rejected built-in, want an error rather than a fallback")
	}
	for _, want := range []string{"tokyo-night", string(theme.ReasonMissingTokens), rejection.Detail} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not carry %q", err.Error(), want)
		}
	}
	if got != (theme.Theme{}) {
		t.Error("resolveTheme returned a palette alongside its error")
	}
}

type rejectingLoader struct {
	rejection *theme.Rejection
}

func (l rejectingLoader) LoadBuiltin(string) (theme.Result, *theme.Rejection, bool) {
	return theme.Result{}, l.rejection, true
}

func TestResolveTheme_PathContentReasonsAreHardErrors(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string) string
		wantReason theme.Reason
	}{
		{
			name:       "a duplicate key",
			wantReason: theme.ReasonBadSyntax,
			setup: func(t *testing.T, dir string) string {
				return themetest.Write(t, dir, "broken.theme", append(themetest.Lines(), "text.primary = #010203"))
			},
		},
		{
			name:       "a bad hex",
			wantReason: theme.ReasonBadColour,
			setup: func(t *testing.T, dir string) string {
				return themetest.WriteWithCanvas(t, dir, "broken.theme", "blue")
			},
		},
		{
			name:       "a missing token",
			wantReason: theme.ReasonMissingTokens,
			setup: func(t *testing.T, dir string) string {
				return themetest.Write(t, dir, "broken.theme", themetest.WithoutKey(themetest.Lines(), "bg.subtle"))
			},
		},
		{
			name:       "an absent file",
			wantReason: theme.ReasonUnreadable,
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "absent.theme")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t, t.TempDir())

			got, err := resolveTheme(theme.NewSilentLoader(), path, io.Discard)

			if err == nil {
				t.Fatalf("resolveTheme(%q) returned nil error, want the hard error %q", path, tt.wantReason)
			}
			if !strings.Contains(err.Error(), string(tt.wantReason)) {
				t.Errorf("error %q does not carry the rejection reason %q", err.Error(), tt.wantReason)
			}
			if got != (theme.Theme{}) {
				t.Error("resolveTheme returned a palette alongside its error")
			}
		})
	}
}

func TestResolveTheme_NoFallbackOnFailure(t *testing.T) {
	dir := t.TempDir()

	args := []struct {
		name string
		arg  string
	}{
		{name: "an unknown slug", arg: "not-a-theme"},
		{name: "a broken file", arg: themetest.WriteWithCanvas(t, dir, "broken.theme", "blue")},
		{name: "an absent file", arg: filepath.Join(dir, "absent.theme")},
	}

	fixtures := []string{"sessions-flat", capture.ContrastValidationFixture}

	for _, ta := range args {
		t.Run(ta.name, func(t *testing.T) {
			got, err := resolveTheme(theme.NewSilentLoader(), ta.arg, io.Discard)
			if err == nil {
				t.Fatalf("resolveTheme(%q) returned nil error", ta.arg)
			}
			if got != (theme.Theme{}) {
				t.Errorf("resolveTheme(%q) fell back to %+v, want the zero palette", ta.arg, got)
			}

			for _, fixture := range fixtures {
				m, err := resolveProgram(fixture, ta.arg, io.Discard)
				if err == nil {
					t.Errorf("resolveProgram(%s, %q) returned nil error", fixture, ta.arg)
				}
				if m != nil {
					t.Errorf("resolveProgram(%s, %q) returned a model alongside its error — nothing must render", fixture, ta.arg)
				}
			}
		})
	}
}

func TestResolveTheme_PathBadNameWarnsWithoutBlocking(t *testing.T) {
	tests := []struct {
		name string
		base string
	}{
		{name: "an upper-case stem", base: "Nord.THEME"},
		{name: "an unexpected extension", base: "mytheme.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := themetest.Write(t, t.TempDir(), tt.base, themetest.Lines())
			var warnings bytes.Buffer

			got, err := resolveTheme(theme.NewSilentLoader(), path, &warnings)

			if err != nil {
				t.Fatalf("resolveTheme(%q) = %v, want the theme rendered despite its name", path, err)
			}
			if got == (theme.Theme{}) {
				t.Error("resolveTheme returned the zero palette for a valid file")
			}
			want := fmt.Sprintf("capturetool: warning: %s: bad name — a themes-directory file must be named <slug>.theme, lowercase (rendering it anyway)\n", tt.base)
			if warnings.String() != want {
				t.Errorf("warnings = %q, want %q", warnings.String(), want)
			}
		})
	}
}

func TestResolveTheme_PathReservedNameWarnsWithoutBlocking(t *testing.T) {
	const base = "nord.theme"
	if !slices.Contains(theme.BuiltinSlugs(), "nord") {
		t.Fatalf("nord is not a built-in (%v) — the fixture cannot collide", theme.BuiltinSlugs())
	}

	path := themetest.Write(t, t.TempDir(), base, themetest.Lines())
	var warnings bytes.Buffer

	got, err := resolveTheme(theme.NewSilentLoader(), path, &warnings)

	if err != nil {
		t.Fatalf("resolveTheme(%q) = %v, want the theme rendered despite its name", path, err)
	}
	if got == (theme.Theme{}) {
		t.Error("resolveTheme returned the zero palette for a valid file")
	}
	builtin := themetest.Builtin(t, "nord")
	if got == builtin {
		t.Error("resolveTheme rendered the built-in nord palette, want the file's own — the candidate slug is not identity")
	}
	want := "capturetool: warning: nord.theme: reserved name — \"nord\" is a built-in slug, so a file with this name is ignored in the themes directory (rendering it anyway)\n"
	if warnings.String() != want {
		t.Errorf("warnings = %q, want %q", warnings.String(), want)
	}
}

func TestResolveTheme_SlugFormNeverWarns(t *testing.T) {
	for _, slug := range theme.BuiltinSlugs() {
		t.Run(slug, func(t *testing.T) {
			var warnings bytes.Buffer

			if _, err := resolveTheme(theme.NewSilentLoader(), slug, &warnings); err != nil {
				t.Fatalf("resolveTheme(%s): %v", slug, err)
			}

			if warnings.Len() != 0 {
				t.Errorf("--theme %s warned %q, want silence — a slug names a built-in by design", slug, warnings.String())
			}
		})
	}
}

func TestResolveModel(t *testing.T) {
	pinned := themetest.Builtin(t, defaultThemeSlug)

	t.Run("known fixture builds a sessions-page model", func(t *testing.T) {
		m, err := resolveModel("sessions-flat", pinned)
		if err != nil {
			t.Fatalf("resolveModel(sessions-flat): %v", err)
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})

	t.Run("unknown fixture is an error that lists the available fixtures", func(t *testing.T) {
		_, err := resolveModel("nope", pinned)
		if err == nil {
			t.Fatal("resolveModel(nope) returned nil error, want error")
		}
		if !strings.Contains(err.Error(), "sessions-flat") {
			t.Errorf("error %q does not list the available fixtures", err.Error())
		}
	})

	t.Run("empty fixture name is an error", func(t *testing.T) {
		if _, err := resolveModel("", pinned); err == nil {
			t.Fatal("resolveModel(\"\") returned nil error, want error")
		}
	})
}

func TestResolveModel_PassesConstantNomination(t *testing.T) {
	pinned := pathThemeForTest(t, "#1A2B3C")

	m, err := resolveModel("sessions-flat", pinned)
	if err != nil {
		t.Fatalf("resolveModel(sessions-flat): %v", err)
	}

	got := m.View().BackgroundColor
	if got == nil {
		t.Fatal("the first frame painted no canvas — the nomination is not constant (an adaptive pair holds the blank frame)")
	}
	if want := pinned.Canvas.Color(); got != want {
		t.Errorf("canvas = %v, want the pinned theme's %v", got, want)
	}
}

func TestResolveModel_NoColorWinsOverTheme(t *testing.T) {
	pinned := pathThemeForTest(t, "#1A2B3C")

	t.Run("NO_COLOR renders the colourless native-bg frame", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		m, err := resolveModel("sessions-flat", pinned)
		if err != nil {
			t.Fatalf("resolveModel(sessions-flat): %v", err)
		}

		if bg := m.View().BackgroundColor; bg != nil {
			t.Errorf("View().BackgroundColor = %v, want nil — NO_COLOR sets no canvas", bg)
		}
		if content := m.View().Content; strings.Contains(content, "48;2;") {
			t.Errorf("the colourless frame emits a background SGR:\n%q", content)
		}
	})

	t.Run("without NO_COLOR the pinned theme paints", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		if err := os.Unsetenv("NO_COLOR"); err != nil {
			t.Fatalf("unset NO_COLOR: %v", err)
		}

		m, err := resolveModel("sessions-flat", pinned)
		if err != nil {
			t.Fatalf("resolveModel(sessions-flat): %v", err)
		}

		if got, want := m.View().BackgroundColor, pinned.Canvas.Color(); got != want {
			t.Errorf("canvas = %v, want the pinned theme's %v", got, want)
		}
	})
}

func TestResolveModel_EnvironmentCannotRecolourAColourlessFixture(t *testing.T) {
	pinned := pathThemeForTest(t, "#1A2B3C")
	t.Setenv("NO_COLOR", "")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatalf("unset NO_COLOR: %v", err)
	}

	t.Run("the colourless fixture renders colourless without NO_COLOR", func(t *testing.T) {
		m, err := resolveModel("sessions-pending-resume-colourless", pinned)
		if err != nil {
			t.Fatalf("resolveModel(sessions-pending-resume-colourless): %v", err)
		}
		if bg := m.View().BackgroundColor; bg != nil {
			t.Errorf("View().BackgroundColor = %v, want nil — the fixture declares NO_COLOR itself", bg)
		}
		if content := m.View().Content; strings.Contains(content, "48;2;") {
			t.Errorf("the colourless fixture emits a background SGR:\n%q", content)
		}
	})

	t.Run("its coloured sibling still paints the pinned theme", func(t *testing.T) {
		m, err := resolveModel("sessions-pending-resume", pinned)
		if err != nil {
			t.Fatalf("resolveModel(sessions-pending-resume): %v", err)
		}
		if got, want := m.View().BackgroundColor, pinned.Canvas.Color(); got != want {
			t.Errorf("canvas = %v, want the pinned theme's %v", got, want)
		}
	})
}

func TestResolveProgram_ThemeDrivesBothBranches(t *testing.T) {
	path := themetest.WriteWithCanvas(t, t.TempDir(), "mytheme.theme", "#1a2b3c")

	pinned, err := resolveTheme(theme.NewSilentLoader(), path, io.Discard)
	if err != nil {
		t.Fatalf("resolveTheme(%q): %v", path, err)
	}

	fixture, err := resolveProgram("sessions-flat", path, io.Discard)
	if err != nil {
		t.Fatalf("resolveProgram(sessions-flat, %q): %v", path, err)
	}
	if got, want := fixture.(tui.Model).View().BackgroundColor, pinned.Canvas.Color(); got != want {
		t.Errorf("the fixture canvas = %v, want the resolved theme's %v", got, want)
	}

	swatch, err := resolveProgram(capture.ContrastValidationFixture, path, io.Discard)
	if err != nil {
		t.Fatalf("resolveProgram(contrast-validation, %q): %v", path, err)
	}
	title := fmt.Sprintf("CONTRAST VALIDATION — canvas %s", pinned.Canvas.Value)
	if content := swatch.View().Content; !strings.Contains(content, title) {
		t.Errorf("the swatch does not render the resolved theme: no %q in its view\n--- view ---\n%s", title, content)
	}
}

func TestCaptureTool_ThemeResolutionIsSilent(t *testing.T) {
	sink := logtest.Install(t)

	dir := t.TempDir()
	reserved := themetest.Write(t, dir, "nord.theme", themetest.Lines())
	broken := themetest.WriteWithCanvas(t, dir, "broken.theme", "blue")

	if _, err := resolveProgram(capture.ContrastValidationFixture, defaultThemeSlug, io.Discard); err != nil {
		t.Fatalf("resolveProgram(contrast-validation, %s): %v", defaultThemeSlug, err)
	}
	if _, err := resolveProgram(capture.ContrastValidationFixture, reserved, io.Discard); err != nil {
		t.Fatalf("resolveProgram(contrast-validation, %s): %v", reserved, err)
	}
	if _, err := resolveProgram(capture.ContrastValidationFixture, "not-a-theme", io.Discard); err == nil {
		t.Fatal("resolveProgram with an unknown theme returned nil error")
	}
	if _, err := resolveProgram(capture.ContrastValidationFixture, broken, io.Discard); err == nil {
		t.Fatal("resolveProgram with a broken theme file returned nil error")
	}

	if got := sink.Records(); len(got) != 0 {
		t.Errorf("theme resolution emitted %d log records, want 0: %+v", len(got), got)
	}

	theme.NewEventLogger(log.For("theme")).Rejected("some-theme", "", &theme.Rejection{Reason: theme.ReasonBadSyntax})
	if got := sink.Records(); len(got) != 1 {
		t.Fatalf("the positive control emitted %d records, want 1 — the sink is not wired to the theme component", len(got))
	}
}

func pathThemeForTest(t *testing.T, canvas string) theme.Theme {
	t.Helper()

	path := themetest.WriteWithCanvas(t, t.TempDir(), "mytheme.theme", canvas)
	pinned, err := resolveTheme(theme.NewSilentLoader(), path, io.Discard)
	if err != nil {
		t.Fatalf("resolveTheme(%q): %v", path, err)
	}
	return pinned
}
