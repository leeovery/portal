package capture_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func remainingPanelFixtureNames() []string {
	return []string{
		panelFixtureNamePrefix + "invalid-row",
		panelFixtureNamePrefix + "dir-unreadable",
		panelFixtureNamePrefix + "narrow",
		panelFixtureNamePrefix + "paginated",
		panelFixtureNamePrefix + "projects",
	}
}

func allPanelFixtureNames() []string {
	names := append(capturePanelFixtureNames(), remainingPanelFixtureNames()...)
	return append(names, messagePanelFixtureNames()...)
}

const (
	minimumPanelTermWidth = 28

	dirUnreadablePanelTermHeight = 18
)

func panelFrameAt(t *testing.T, fixture string, palette theme.Theme, w, h int) string {
	t.Helper()

	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		t.Fatalf("FixtureByName(%s): %v", fixture, err)
	}
	return fx.ModelAt(palette, w, h).View().Content
}

func fgSeq(t *testing.T, tok theme.Token) string {
	t.Helper()
	return sgrParameterRun(t, lipgloss.NewStyle().Foreground(tok.Color()))
}

func panelOuterWidth(t *testing.T, frame string, termW int) int {
	t.Helper()

	for line := range strings.SplitSeq(ansi.Strip(frame), "\n") {
		page, _, onPanel := strings.Cut(line, panelBorder)
		if !onPanel {
			continue
		}
		return termW - tui.Hinset - lipgloss.Width(page)
	}
	t.Fatalf("no frame line carries the panel's left border %q, so the panel did not render:\n%s", panelBorder, ansi.Strip(frame))
	return 0
}

func TestPanelFixture_InvalidRowFrame(t *testing.T) {
	palette := themetest.Builtin(t, "tokyo-night")
	frame := panelFixtureFrame(t, "theme-panel-invalid-row", palette)
	rows := panelRows(t, frame)

	t.Run("the reason-only row renders the glyph and the reason on one line", func(t *testing.T) {
		if got, want := rows["aurora-glow"].badge, "⚠ bad syntax"; got != want {
			t.Errorf("the aurora-glow row's trailing elements = %q, want %q", got, want)
		}
		_, line := panelRowLine(t, frame, "aurora-glow")
		if !strings.Contains(ansi.Strip(line), "bad syntax") {
			t.Errorf("the label and its reason are not on the same line: %q", ansi.Strip(line))
		}
	})

	t.Run("the invalid label is text.subtle and its reason accent.attention", func(t *testing.T) {
		_, line := panelRowLine(t, frame, "aurora-glow")
		if !strings.Contains(line, fgSeq(t, palette.TextSubtle)) {
			t.Errorf("the aurora-glow row does not carry text.subtle; an invalid label is de-emphasised BUT READABLE, and text.faint must never reach the UI floor:\n%q", ansi.Strip(line))
		}
		if !strings.Contains(line, fgSeq(t, palette.AccentAttention)) {
			t.Errorf("the aurora-glow row does not carry accent.attention; the `⚠` and its reason are one accent.attention run:\n%q", ansi.Strip(line))
		}
	})

	t.Run("the persisted invalid row keeps its badge and loses its reason", func(t *testing.T) {
		if got := rows["nord-lee"].badge; !strings.Contains(got, "● dark") {
			t.Errorf("the nord-lee row's trailing elements = %q, want the `● dark` badge — the badge marks what is SET, and a fallback never moves it", got)
		}
		if got := rows["nord-lee"].badge; strings.Contains(got, "bad colour") {
			t.Errorf("the nord-lee row still renders its reason (%q); the badge outranks the reason, so a badged row has no reason slot to fill", got)
		}
	})

	t.Run("the over-long label is truncated with an ellipsis", func(t *testing.T) {
		visible := ansi.Strip(frame)
		if !strings.Contains(visible, "My Gorgeous Midnight Pa…") {
			t.Errorf("the over-long `bad name` label is not truncated to the panel's budget:\n%s", visible)
		}
		if strings.Contains(visible, "My Gorgeous Midnight Palette.theme") {
			t.Error("the over-long label renders in full; it must be truncated with `…` to whatever the fixed columns leave")
		}
	})

	t.Run("the cursor rests on a valid row", func(t *testing.T) {
		if !rows["tokyo-night"].cursor {
			t.Error("the cursor is not on tokyo-night; the arrow skip keeps it off invalid rows, so a frame with it parked on one is a state production cannot reach")
		}
		for _, invalid := range []string{"aurora-glow", "nord-lee"} {
			if rows[invalid].cursor {
				t.Errorf("the cursor is on the invalid row %s", invalid)
			}
		}
	})
}

func TestPanelFixture_InvalidPersistedRowDropsTheReason(t *testing.T) {
	visible := ansi.Strip(panelFixtureFrame(t, "theme-panel-invalid-row", themetest.Builtin(t, "tokyo-night")))

	if !strings.Contains(visible, "bad syntax") {
		t.Errorf("the UNBADGED invalid row renders no reason at all, so a missing reason on the badged row demonstrates nothing about the priority:\n%s", visible)
	}
	if strings.Contains(visible, "bad colour") {
		t.Errorf("the BADGED invalid row still renders its reason; the marker must always have a home, so the badge takes the right edge and the reason is dropped:\n%s", visible)
	}
}

func TestPanelFixture_DirUnreadableIsChromeOnPageTwo(t *testing.T) {
	frame := panelFrameAt(t, "theme-panel-dir-unreadable", themetest.Builtin(t, "tokyo-night"), harnessWidth, dirUnreadablePanelTermHeight)
	visible := ansi.Strip(frame)

	if strings.Contains(visible, "nord-lee") {
		t.Fatalf("the frame still shows the page-1 rows, so the `Ctrl+↓` never paged and this asserts nothing about chrome:\n%s", visible)
	}

	dirRow, _ := panelLineWith(t, frame, "dir unreadable")
	if !strings.Contains(visible, "⚠ dir unreadable") {
		t.Errorf("the pinned copy is not rendered verbatim; it is fixed at 16 columns precisely so it never truncates:\n%s", visible)
	}

	header, _ := panelLineWith(t, frame, "Themes")
	if dirRow <= header {
		t.Errorf("the directory row is on line %d and the header on line %d; it is pinned DIRECTLY BENEATH the header", dirRow, header)
	}
	for _, row := range []string{"solarized-lee", "tokyo-night"} {
		at, _ := panelRowLine(t, frame, row)
		if at <= dirRow {
			t.Errorf("theme row %s renders on line %d, above the pinned directory row on line %d", row, at, dirRow)
		}
	}
}

func TestPanelFixture_RowsBeneathDirRow(t *testing.T) {
	frame := panelFrameAt(t, "theme-panel-dir-unreadable", themetest.Builtin(t, "tokyo-night"), harnessWidth, dirUnreadablePanelTermHeight)
	rows := panelRows(t, frame)

	t.Run("a built-in row renders beneath it", func(t *testing.T) {
		if !rows["tokyo-night"].cursor {
			t.Errorf("the built-in row tokyo-night does not carry the cursor; the parsed rows are %v", labelSet(rows))
		}
	})

	t.Run("a persisted-slug row keeps its badge beneath it", func(t *testing.T) {
		if got := rows["solarized-lee"].badge; !strings.Contains(got, "● light") {
			t.Errorf("the persisted row solarized-lee's trailing elements = %q, want the `● light` badge — losing it is the regression this pins", got)
		}
	})
}

func TestPanelFixture_NarrowRendersTheStepDown(t *testing.T) {
	palette := themetest.Builtin(t, "nord")

	// The ladder's ends are measured off the fixture this one is derived from,
	// which declares no size of its own and so renders at whatever is asked for.
	preferred := panelOuterWidth(t, panelFixtureFrame(t, "theme-panel-adaptive-pair", palette), harnessWidth)
	minimum := panelOuterWidth(t, panelFrameAt(t, "theme-panel-adaptive-pair", palette, minimumPanelTermWidth, harnessHeight), minimumPanelTermWidth)
	if minimum >= preferred {
		t.Fatalf("the ladder's ends measure %d (minimum) and %d (preferred); with no step between them the assertion below cannot fail", minimum, preferred)
	}

	narrowTermWidth := tui.ThemePanelMinWidthTerminal()
	narrow := panelFixtureFrame(t, "theme-panel-narrow", palette)
	got := panelOuterWidth(t, narrow, narrowTermWidth)
	if got != minimum {
		t.Errorf("at its declared %d columns the panel renders %d wide, want the ladder's stepped-down %d (the preferred width is %d)", narrowTermWidth, got, minimum, preferred)
	}

	t.Run("every row still renders on exactly one line", func(t *testing.T) {
		for _, slug := range panelUnionSlugs() {
			panelSlugRow(t, narrow, slug)
		}
	})

	t.Run("the badges survive the narrowed width", func(t *testing.T) {
		if got, want := panelSlugRow(t, narrow, "nord").badge, "● dark"; got != want {
			t.Errorf("the nord row's badge = %q, want %q — the badge is reserved before anything can crowd it out", got, want)
		}
		if got, want := panelSlugRow(t, narrow, "tokyo-night-day").badge, "● light"; got != want {
			t.Errorf("the tokyo-night-day row's badge = %q, want %q", got, want)
		}
	})
}

func TestPanelFixture_PaginatedDrawsDots(t *testing.T) {
	palette := themetest.Builtin(t, "nord")
	frame := panelFixtureFrame(t, "theme-panel-paginated", palette)

	t.Run("a four-row panel draws no dots at the same size", func(t *testing.T) {
		if panelCarriesDots(t, panelFixtureFrame(t, "theme-panel-adaptive-pair", palette)) {
			t.Error("the four-row panel already draws pagination dots, so drawing them on the paginating one demonstrates nothing about overflow")
		}
	})

	t.Run("the overflowing panel draws them", func(t *testing.T) {
		if !panelCarriesDots(t, frame) {
			t.Errorf("the panel draws no pagination dots, so the swap-and-diff guard is blind at the panel's bubbles/list instance:\n%s", ansi.Strip(frame))
		}
	})

	t.Run("the dots are painted from the active theme", func(t *testing.T) {
		_, line := panelLineWith(t, frame, paginationDot)
		if !strings.Contains(line, fgSeq(t, palette.AccentPrimary)) {
			t.Error("the ACTIVE dot is not accent.primary; `bubbles/list` reads the dot strings out of its styles once at construction, so this is the site a restyle must re-feed")
		}
		if !strings.Contains(line, fgSeq(t, palette.TextFaint)) {
			t.Error("no INACTIVE dot is text.faint; a single-dot row is not a paginating one")
		}
	})
}

const paginationDot = "•"

func panelCarriesDots(t *testing.T, frame string) bool {
	t.Helper()

	for _, line := range panelLines(t, frame) {
		if strings.Contains(line.visible, paginationDot) {
			return true
		}
	}
	return false
}

func TestPanelFixture_OverProjects(t *testing.T) {
	palette := themetest.Builtin(t, "nord")

	fx, err := capture.FixtureByName("theme-panel-projects")
	if err != nil {
		t.Fatalf("FixtureByName(theme-panel-projects): %v", err)
	}
	model := fx.ModelAt(palette, harnessWidth, harnessHeight)
	frame := model.View().Content
	visible := ansi.Strip(frame)

	t.Run("the page beneath is Projects", func(t *testing.T) {
		if model.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects — the `x` its captureKeys type never landed", model.ActivePage())
		}
		if !strings.Contains(visible, "flow-v1-api") {
			t.Errorf("the frame carries no project rows:\n%s", visible)
		}
	})

	t.Run("the panel is composited over it with its badges", func(t *testing.T) {
		rows := panelRows(t, frame)
		if got, want := rows["nord"].badge, "● dark"; got != want {
			t.Errorf("the nord row's badge = %q, want %q", got, want)
		}
		if got, want := rows["tokyo-night-day"].badge, "● light"; got != want {
			t.Errorf("the tokyo-night-day row's badge = %q, want %q", got, want)
		}
	})

	t.Run("the Projects footer beneath is cut by the overlay", func(t *testing.T) {
		plain := ansi.Strip(panelFixtureFrame(t, "projects", palette))
		bare, covered := footerLine(t, plain), footerLine(t, visible)
		if bare == covered {
			t.Fatalf("the footer is identical with and without the panel, so nothing was covered:\n%s", covered)
		}
		if !strings.HasPrefix(bare, covered) {
			t.Errorf("the covered footer %q is not a prefix cut of the bare footer %q; the overlay cuts wherever its border falls rather than reflowing", covered, bare)
		}
	})
}

func footerLine(t *testing.T, visible string) string {
	t.Helper()
	for line := range strings.SplitSeq(visible, "\n") {
		if !strings.Contains(line, "new session") {
			continue
		}
		page, _, _ := strings.Cut(line, panelBorder)
		return strings.TrimRight(page, " ")
	}
	t.Fatalf("no line carries the Projects footer:\n%s", visible)
	return ""
}

func TestPanelFixture_RegistryHoldsTheSpecifiedPanelSet(t *testing.T) {
	panels := registeredPanelFixtureNames()
	want := allPanelFixtureNames()
	slices.Sort(panels)
	slices.Sort(want)
	if !slices.Equal(panels, want) {
		t.Errorf("the registry holds panel fixtures %v, want exactly the specified set %v", panels, want)
	}
}
