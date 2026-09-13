package cmd

import (
	"go/ast"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func themeSourceForTest(t *testing.T) (theme.Loader, *logtest.Sink) {
	t.Helper()
	sink := logtest.Install(t)
	return newThemeLoader(), sink
}

func pressThemeKeyOnModel(t *testing.T, m tui.Model) string {
	t.Helper()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	model, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	return model.View().Content
}

// Restated rather than reached for: internal/tui holds the label unexported.
const themePanelHeaderCopy = "Themes"

func TestThemeSource_ReadsOnlyWhenOpened(t *testing.T) {
	dir := useThemesDir(t)
	writeThemeFile(t, dir, "sunset", "#101010")
	loader, sink := themeSourceForTest(t)

	enumerator := newThemeSource(loader)

	if got := themeEventCount(t, sink, "enumerated"); got != 0 {
		t.Errorf("constructing the adapter emitted %d `theme: enumerated` records, want 0 — the read is on the keypress", got)
	}

	const opens = 3
	for i := range opens {
		_, union := enumerator.Open(theme.RawKeys{})
		if !slugListed(union, "sunset") {
			t.Fatalf("open %d produced no row for the drop-in (rows: %v) — the adapter must read the resolved themes directory", i+1, unionSlugs(union))
		}
	}

	if got := themeEventCount(t, sink, "enumerated"); got != opens {
		t.Errorf("%d opens emitted %d `theme: enumerated` records, want %d (one per open event, no dedup)", opens, got, opens)
	}
}

func TestThemeSource_ReassembleDoesNoIO(t *testing.T) {
	dir := useThemesDir(t)
	writeThemeFile(t, dir, "sunset", "#101010")
	loader, sink := themeSourceForTest(t)
	enumerator := newThemeSource(loader)

	enumeration, _ := enumerator.Open(theme.RawKeys{})
	before := themeEventCount(t, sink, "enumerated")

	union := enumerator.Reassemble(enumeration, theme.RawKeys{Theme: "sunset"})

	if !slugListed(union, "sunset") {
		t.Errorf("Reassemble dropped the retained drop-in (rows: %v)", unionSlugs(union))
	}
	if got := themeEventCount(t, sink, "enumerated"); got != before {
		t.Errorf("Reassemble emitted %d further `theme: enumerated` records, want 0 — it performs no I/O", got-before)
	}
}

// A Loader owns the `theme` component's per-process dedup state, so a panel
// handed a second loader would WARN twice about one misconfiguration.
func TestThemeSource_SharesTheConstructionReadsDedupScope(t *testing.T) {
	// A drop-in slug: a built-in would resolve out of the embedded set and
	// never consult the directory.
	const dropIn = "a-drop-in"

	openAfterConstruction := func(t *testing.T, panelLoader func(theme.Loader) theme.Loader) int {
		t.Helper()
		// An existing but unreadable themes directory earns a `theme: directory
		// unusable` WARN, where an absent one is silent.
		_ = themetest.DenyDir(t, useThemesDir(t))
		sink := logtest.Install(t)
		construction := newThemeLoader()

		if _, _, err := themeResolution(prefs.ThemeKeys{Theme: dropIn}, construction); err != nil {
			t.Fatalf("construction-time resolution: %v", err)
		}
		if got := themeEventCount(t, sink, "directory unusable"); got != 1 {
			t.Fatalf("precondition: the construction read emitted %d `theme: directory unusable` records, want 1", got)
		}

		newThemeSource(panelLoader(construction)).Open(theme.RawKeys{Theme: dropIn})
		return themeEventCount(t, sink, "directory unusable")
	}

	t.Run("the construction loader dedups the panel's repeat", func(t *testing.T) {
		got := openAfterConstruction(t, func(l theme.Loader) theme.Loader { return l })
		if got != 1 {
			t.Errorf("`theme: directory unusable` was emitted %d times, want 1 — the panel must share the construction read's dedup scope", got)
		}
	})

	t.Run("a second loader would not", func(t *testing.T) {
		got := openAfterConstruction(t, func(theme.Loader) theme.Loader { return newThemeLoader() })
		if got != 2 {
			t.Fatalf("a fresh loader emitted %d records, want 2 — the control does not demonstrate the duplicate the shared loader prevents", got)
		}
	})
}

// Building a second loader is silent — everything works, the user just gets
// every theme WARN twice per launch — and openTUI ends in a running Bubble Tea
// program, so only a source guard reaches it.
func TestOpenTUI_BuildsOneThemeLoader(t *testing.T) {
	fn := funcDeclForTest(t, "open.go", "openTUI")

	for _, constructor := range []string{"buildThemeLoader", "newThemeLoader"} {
		if got := callCount(fn, constructor); got > 1 {
			t.Errorf("openTUI calls %s %d times, want at most 1 — one loader per launch is one dedup scope per launch", constructor, got)
		}
	}

	assertThemeSourceTakesBoundLoader(t, fn)
}

// Asserts the argument's shape, not its identity: any call expression in that
// position is a loader built for the panel alone.
func assertThemeSourceTakesBoundLoader(t *testing.T, n ast.Node) {
	t.Helper()
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); !isIdent || ident.Name != "newThemeSource" || len(call.Args) != 1 {
			return true
		}
		found = true
		if _, isIdent := call.Args[0].(*ast.Ident); !isIdent {
			t.Errorf("openTUI hands newThemeSource a freshly-constructed loader; it must share the construction-time instance")
		}
		return true
	})
	if !found {
		t.Fatal("no single-argument newThemeSource call found; the guard would pass vacuously")
	}
}

func callCount(n ast.Node, name string) int {
	count := 0
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == name {
			count++
		}
		return true
	})
	return count
}

// Nothing is injected for the `●`, so the marker exists only if the panel
// reached themeSource.Resolve and got the constant back.
func TestThemePanelOpen_WiredThroughBuildTUIModel(t *testing.T) {
	dir := useThemesDir(t)
	writeThemeFile(t, dir, "sunset", "#101010")
	loader, _ := themeSourceForTest(t)

	cfg := defaultTestTUIConfig()
	cfg.themeSource = newThemeSource(loader)
	cfg.themeKeys = theme.RawKeys{Theme: "sunset"}

	view := pressThemeKeyOnModel(t, buildTUIModel(cfg, pickerLanding{}, nil))

	for _, want := range []string{themePanelHeaderCopy, "sunset", "●"} {
		if !strings.Contains(view, want) {
			t.Errorf("the frame after `t` does not contain %q:\n%s", want, view)
		}
	}
}

// The poisoned (mode 0000) directory makes any read loud: an unusable directory
// earns a `theme: directory unusable` WARN, where an absent one is silent.
func TestThemePanelOpen_ExecPathUntouched(t *testing.T) {
	_ = themetest.DenyDir(t, useThemesDir(t))
	// A drop-in slug: a built-in would never touch the poison.
	setPrefsFile(t, `{"theme":"a-drop-in"}`)

	loud := logtest.Install(t)
	newThemeSource(newThemeLoader()).Open(theme.RawKeys{Theme: "a-drop-in"})
	if len(themeEvents(t, loud)) == 0 {
		t.Fatal("the panel seam emitted no theme record against the poisoned directory; the zero-record assertion below would be vacuous")
	}

	sink := logtest.Install(t)

	if got := execOpenSession(t, "api-x7Kd9a"); got != "api-x7Kd9a" {
		t.Fatalf("open attached %q, want the session it resolved — the exec path must have run", got)
	}

	assertThemeEvents(t, sink)
}

func themeEventCount(t *testing.T, sink *logtest.Sink, msg string) int {
	t.Helper()
	count := 0
	for _, rec := range sink.Records() {
		if rec.Msg == msg && rec.HasAttr("component") && rec.AttrString(t, "component") == "theme" {
			count++
		}
	}
	return count
}

func slugListed(union theme.Union, slug string) bool {
	for _, row := range union.Rows {
		if row.Slug == slug {
			return true
		}
	}
	return false
}

func unionSlugs(union theme.Union) []string {
	labels := make([]string, 0, len(union.Rows))
	for _, row := range union.Rows {
		labels = append(labels, row.Label())
	}
	return labels
}
