package capture_test

import (
	"go/ast"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

const (
	harnessWidth  = 120
	harnessHeight = 40
)

func buildBackedFixtureNames() []string {
	names := capture.FixtureNames()
	return slices.DeleteFunc(names, func(n string) bool { return n == capture.ContrastValidationFixture })
}

type capturedStateWant struct {
	fixture string
	page    any
	present []string
	absent  []string
}

func capturedStates() []capturedStateWant {
	sessionRow := "agentic-workflows-code-based"
	return []capturedStateWant{
		{fixture: "sessions-flat", page: tui.PageSessions, present: []string{sessionRow}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-empty", page: tui.PageSessions, present: []string{"No sessions yet"}},
		{fixture: "sessions-by-project", page: tui.PageSessions, present: []string{sessionRow, "— by project"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-by-tag", page: tui.PageSessions, present: []string{sessionRow, "— by tag"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-paged", page: tui.PageSessions, present: []string{"session-00"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-inline-flash", page: tui.PageSessions, present: []string{"fab-flowx-explore", "closed externally"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-rename-refused-separator", page: tui.PageSessions, present: []string{sessionRow, `":" isn't allowed in a session name — tmux reads it as a separator`}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-rename-refused-id-prefix", page: tui.PageSessions, present: []string{sessionRow, `"$" isn't allowed at the start of a session name — tmux reads it as a session ID`}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-rename-refused-flag-prefix", page: tui.PageSessions, present: []string{sessionRow, `"-" isn't allowed at the start of a session name — tmux reads it as a command flag`}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-multi-select-active", page: tui.PageSessions, present: []string{sessionRow, "3 selected"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-unsupported-terminal", page: tui.PageSessions, present: []string{sessionRow, "unsupported terminal", "com.apple.Terminal"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-unsupported-null", page: tui.PageSessions, present: []string{sessionRow}, absent: []string{"No sessions yet", "unsupported terminal"}},
		{fixture: "sessions-multi-select-preflight-abort", page: tui.PageSessions, present: []string{sessionRow, "is gone", "nothing opened"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-burst-opening", page: tui.PageSessions, present: []string{sessionRow, "Opening 2/3"}, absent: []string{"No sessions yet"}},
		{fixture: "sessions-no-tags-signpost", page: tui.PageSessions, present: []string{"fab-flowx-explore", "No tags yet"}, absent: []string{"No sessions yet"}},
		{fixture: "theme-panel-adaptive-pair", page: tui.PageSessions, present: []string{sessionRow, "Themes", "● light", "● dark"}, absent: []string{"No sessions yet"}},
		{fixture: "theme-panel-constant-previewing", page: tui.PageSessions, present: []string{sessionRow, "Themes", "●"}, absent: []string{"No sessions yet", "● light", "● dark"}},
		{fixture: "theme-panel-invalid-row", page: tui.PageSessions, present: []string{sessionRow, "Themes", "⚠ bad syntax", "My Gorgeous Midnight Pa…", "● dark"}, absent: []string{"No sessions yet", "bad colour"}},
		{fixture: "theme-panel-dir-unreadable", page: tui.PageSessions, present: []string{sessionRow, "Themes", "⚠ dir unreadable", "solarized-lee", "● light", "● dark"}, absent: []string{"No sessions yet"}},
		{fixture: "theme-panel-narrow", page: tui.PageSessions, present: []string{sessionRow, "Themes", "● light", "● dark"}, absent: []string{"No sessions yet"}},
		{fixture: "theme-panel-paginated", page: tui.PageSessions, present: []string{sessionRow, "Themes", "vivid-01", "● light", "● dark"}, absent: []string{"No sessions yet"}},
		{fixture: "theme-panel-projects", page: tui.PageProjects, present: []string{"flow-v1-api", "Projects", "Themes", "● light", "● dark"}},
		{fixture: "theme-panel-confirm", page: tui.PageSessions, present: []string{sessionRow, "Themes", "clear constant nord?  y / n", "confirm", "cancel"}, absent: []string{"No sessions yet", "set as dark", "set as light"}},
		{fixture: "theme-panel-commit-failed", page: tui.PageSessions, present: []string{sessionRow, "Themes", "⚠ couldn't save theme", "● light", "● dark", "set as dark"}, absent: []string{"No sessions yet", "confirm", "cancel"}},
		// The floor's body is one list row, so the light badge's row is off the
		// frame rather than missing from it.
		{fixture: "theme-panel-min-height-message", page: tui.PageSessions, present: []string{sessionRow, "Themes", "⚠ couldn't save theme", "● dark"}, absent: []string{"No sessions yet", "● light"}},
		{fixture: "projects", page: tui.PageProjects, present: []string{"flow-v1-api", "Projects"}},
		{fixture: "projects-command-pending", page: tui.PageProjects, present: []string{"flow-v1-api", "Pick a project to run", "npm run dev"}},
		{fixture: "preview-screen", present: []string{"aviva-proxy-qNyfEO", "Window 1/1", "kubectl rollout"}},
		{fixture: "loading-screen", page: tui.PageLoading, present: []string{"Restoring sessions", "8", "12"}},
		{fixture: "loading-error", page: tui.PageLoading, present: []string{"✗", "Portal failed to set @portal-restoring marker"}},
	}
}

func darkBuiltinTheme(t *testing.T) theme.Theme {
	t.Helper()
	return themetest.DefaultDark(t)
}

func TestModelAt_ReachesCapturedState(t *testing.T) {
	pinned := darkBuiltinTheme(t)

	t.Run("the table covers every build-backed fixture", func(t *testing.T) {
		listed := buildBackedFixtureNames()
		covered := make([]string, 0, len(capturedStates()))
		for _, want := range capturedStates() {
			covered = append(covered, want.fixture)
		}
		slices.Sort(covered)
		if !slices.Equal(listed, covered) {
			t.Errorf("captured-state table covers %v, want every build-backed fixture %v", covered, listed)
		}
	})

	for _, want := range capturedStates() {
		t.Run(want.fixture, func(t *testing.T) {
			fx, err := capture.FixtureByName(want.fixture)
			if err != nil {
				t.Fatalf("FixtureByName(%s): %v", want.fixture, err)
			}

			m := fx.ModelAt(pinned, harnessWidth, harnessHeight)
			if want.page != nil && any(m.ActivePage()) != want.page {
				t.Errorf("ActivePage() = %d, want %d — the driver did not reach the captured screen", m.ActivePage(), want.page)
			}

			frame := ansi.Strip(m.View().Content)
			for _, s := range want.present {
				if !strings.Contains(frame, s) {
					t.Errorf("captured frame is missing %q:\n%s", s, frame)
				}
			}
			for _, s := range want.absent {
				if strings.Contains(frame, s) {
					t.Errorf("captured frame carries %q, which the captured state must not show:\n%s", s, frame)
				}
			}
		})
	}
}

func lightBuiltinTheme(t *testing.T) theme.Theme {
	t.Helper()
	return themetest.DefaultLight(t)
}

func bgSeq(t *testing.T, tok theme.Token) string {
	t.Helper()
	return sgrParameterRun(t, lipgloss.NewStyle().Background(tok.Color()))
}

func swapPalettes(t *testing.T) (a, b theme.Theme) {
	t.Helper()
	a, b = darkBuiltinTheme(t), lightBuiltinTheme(t)
	if a.Canvas.Value == b.Canvas.Value {
		t.Fatalf("the two palettes share a canvas (%s); a swap between them would be unobservable", a.Canvas.Value)
	}
	return a, b
}

func TestRenderSwapRender_ARenderPopulatesCachesBeforeSwap(t *testing.T) {
	a, b := swapPalettes(t)
	fx, err := capture.FixtureByName("sessions-flat")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-flat): %v", err)
	}

	before, after := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)

	if before == "" {
		t.Fatal("the before-frame is empty; the A-render never happened, so nothing populated the caches")
	}
	visible := strings.TrimSpace(ansi.Strip(before))
	if visible == "" {
		t.Fatalf("the before-frame is the pre-resolution blank frame (all spaces, no content); the constant nomination must paint from frame one:\n%q", before)
	}
	if !strings.Contains(ansi.Strip(before), "agentic-workflows-code-based") {
		t.Errorf("the before-frame does not render the fixture's session rows, so it is not the fixture's captured state:\n%s", ansi.Strip(before))
	}

	aCanvas, bCanvas := bgSeq(t, a.Canvas), bgSeq(t, b.Canvas)
	if !strings.Contains(before, aCanvas) {
		t.Errorf("the before-frame does not carry theme A's canvas %q — it was not painted under A", aCanvas)
	}
	if strings.Contains(before, bCanvas) {
		t.Errorf("the before-frame already carries theme B's canvas %q — the A-render was not under A", bCanvas)
	}
	if !strings.Contains(after, bCanvas) {
		t.Errorf("the after-frame does not carry theme B's canvas %q — the swap did not take", bCanvas)
	}
}

func TestRenderSwapRender_MutatesASingleModel(t *testing.T) {
	a, b := swapPalettes(t)

	t.Run("state established before the swap survives it", func(t *testing.T) {
		fx, err := capture.FixtureByName("projects")
		if err != nil {
			t.Fatalf("FixtureByName(projects): %v", err)
		}

		before, after := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)
		for _, tc := range []struct {
			name  string
			frame string
		}{{"before", before}, {"after", after}} {
			visible := ansi.Strip(tc.frame)
			if !strings.Contains(visible, "Projects 14") {
				t.Errorf("the %s-frame is not on the Projects page; the captured state did not survive:\n%s", tc.name, visible)
			}
		}
	})

	t.Run("exactly one model is constructed", func(t *testing.T) {
		body := harnessFuncBody(t, "RenderSwapRender")
		if got := countCalls(body, "ModelAt"); got != 1 {
			t.Errorf("RenderSwapRender makes %d ModelAt call(s), want exactly 1 — building one model per theme is the vacuous-pass shape", got)
		}
		if got := countCalls(body, "Build"); got != 0 {
			t.Errorf("RenderSwapRender makes %d tui.Build call(s), want 0 — the second frame comes from swapping the first model, never from a second one", got)
		}
		if got := countCalls(body, "ApplyTheme"); got != 1 {
			t.Errorf("RenderSwapRender makes %d ApplyTheme call(s), want exactly 1 — the swap goes through the production entry point", got)
		}
	})
}

func harnessFuncBody(t *testing.T, name string) *ast.BlockStmt {
	t.Helper()
	const harnessFile = "harness.go"
	source := sourceguardtest.PackageSource(t, ".", harnessFile)
	for _, decl := range source.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Body != nil {
			return fn.Body
		}
	}
	t.Fatalf("%s declares no method named %s", harnessFile, name)
	return nil
}

func countCalls(block *ast.BlockStmt, name string) int {
	count := 0
	ast.Inspect(block, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sourceguardtest.CalleeName(call) == name {
			count++
		}
		return true
	})
	return count
}
