package tui

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestThemeFlash_OriginDiscriminator(t *testing.T) {
	t.Run("a fresh model carries the default origin", func(t *testing.T) {
		m := noticeBandModel("alpha-row")

		if m.flashOrigin != flashOriginDefault {
			t.Errorf("a fresh model's flash origin = %v, want the default origin", m.flashOrigin)
		}
	})

	t.Run("setThemeFlash stamps the theme origin", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		gen := m.flashGen

		(&m).setThemeFlash(themePanelNoColorFlash)

		if got := m.flashText; got != themePanelNoColorFlash {
			t.Errorf("flashText = %q, want the raised copy %q", got, themePanelNoColorFlash)
		}
		if m.flashOrigin != flashOriginTheme {
			t.Errorf("flash origin = %v, want the theme origin", m.flashOrigin)
		}
		if m.flashKind != flashWarning {
			t.Errorf("flash kind = %v, want flashWarning — a theme flash is an ordinary warning band", m.flashKind)
		}
		if got, want := m.flashGen, gen+1; got != want {
			t.Errorf("flashGen = %d, want %d — a theme flash bumps the shared generation counter", got, want)
		}
	})

	t.Run("setThemeFlash re-syncs the layout exactly as setFlash does", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		_, base := m.SessionListSize()

		(&m).setThemeFlash(themePanelNoColorFlash)

		slot := m.sessionBandHeight()
		if slot == 0 {
			t.Fatal("a theme flash reserved no band rows; the slot must be measured like every other flash's")
		}
		if _, got := m.SessionListSize(); got != base-slot {
			t.Errorf("session list height with a theme flash = %d, want %d (base %d − slot %d)", got, base-slot, base, slot)
		}
	})

	t.Run("setFlash resets the origin to default", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		(&m).setThemeFlash(themePanelNoColorFlash)

		m.setFlash("__ORDINARY__")

		if m.flashOrigin != flashOriginDefault {
			t.Errorf("flash origin after setFlash = %v, want the default — an ordinary flash never inherits the theme tier", m.flashOrigin)
		}
	})

	t.Run("setSuccessFlash resets the origin to default", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		(&m).setThemeFlash(themePanelNoColorFlash)

		m.setSuccessFlash("__ORDINARY__")

		if m.flashOrigin != flashOriginDefault {
			t.Errorf("flash origin after setSuccessFlash = %v, want the default — an ordinary flash never inherits the theme tier", m.flashOrigin)
		}
		if m.flashKind != flashSuccess {
			t.Errorf("flash kind after setSuccessFlash = %v, want flashSuccess", m.flashKind)
		}
	})

	t.Run("the theme tier claims only a theme-origin flash", func(t *testing.T) {
		m := noticeBandModel("alpha-row")

		if _, _, ok := m.themeFlashClaim(); ok {
			t.Error("the theme tier claimed the slot with no flash live")
		}

		m.setFlash("__ORDINARY__")

		if _, _, ok := m.themeFlashClaim(); ok {
			t.Error("the theme tier claimed an ordinary flash; the tier is scoped to the theme signals")
		}

		(&m).setThemeFlash(themePanelNoColorFlash)

		role, message, ok := m.themeFlashClaim()
		if !ok || role != bandWarning || message != themePanelNoColorFlash {
			t.Errorf("the theme tier = (role %v, message %q, ok %v), want (bandWarning, %q, true)", role, message, ok, themePanelNoColorFlash)
		}
	})
}

func TestThemeFlash_AllSixUseSetThemeFlash(t *testing.T) {
	t.Run("every reachable theme copy carries the theme origin", func(t *testing.T) {
		floorH := themePanelMinHeight(themePanelKeymap(), false)

		for _, tc := range []struct {
			name  string
			raise func(t *testing.T) Model
			want  string
		}{
			{
				name: "the NO_COLOR block",
				raise: func(t *testing.T) Model {
					t.Helper()
					m, _ := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
						page:       PageSessions,
						colourless: true,
						contentW:   entryContentW,
						contentH:   entryContentH,
					})
					m, _ = pressThemeKeyCmd(t, m)
					return m
				},
				want: wantNoColorEntryFlash,
			},
			{
				name: "the width-floor block",
				raise: func(t *testing.T) Model {
					t.Helper()
					m, _ := pressThemeKeyCmd(t, mustEntryModel(t, themePanelMinWidth-1, entryContentH))
					return m
				},
				want: wantNarrowEntryFlash,
			},
			{
				name: "the height-floor block",
				raise: func(t *testing.T) Model {
					t.Helper()
					m, _ := pressThemeKeyCmd(t, mustEntryModel(t, entryContentW, floorH-1))
					return m
				},
				want: wantShortEntryFlash,
			},
			{
				name: "the forced close below the width floor",
				raise: func(t *testing.T) Model {
					t.Helper()
					contentW, contentH := geometryBelowWidthFloor()
					return resizeForTest(t, newGeometryPanelModel(t, geometryWideW, geometryContentH), contentW, contentH)
				},
				want: wantNarrowClosedFlash,
			},
			{
				name: "the forced close below the height floor",
				raise: func(t *testing.T) Model {
					t.Helper()
					contentW, contentH := geometryBelowHeightFloor()
					return resizeForTest(t, newGeometryPanelModel(t, geometryWideW, geometryContentH), contentW, contentH)
				},
				want: wantShortClosedFlash,
			},
			{
				name: "the close report",
				raise: func(t *testing.T) Model {
					t.Helper()
					m, _ := newCloseReportModel(t)
					m, _ = closePanelForTest(t, m)
					return m
				},
				want: wantThemeNotSavedFlash,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.raise(t)

				if got := m.flashText; got != tc.want {
					t.Fatalf("the raised copy is %q, want %q", got, tc.want)
				}
				if m.flashOrigin != flashOriginTheme {
					t.Errorf("%q carries origin %v, want the theme origin — it must be raised through setThemeFlash", tc.want, m.flashOrigin)
				}
			})
		}
	})

	t.Run("no theme copy reaches setFlash directly", func(t *testing.T) {
		parsed := sourceguardtest.ParsePackageSources(t, ".", false)
		vocabulary := themeCopyVocabulary(filesByName(parsed))
		if len(vocabulary) == 0 {
			t.Fatal("the theme-copy vocabulary is empty, so this guard forbids nothing")
		}

		for _, source := range parsed {
			name := filepath.Base(source.Path)
			ast.Inspect(source.File, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "setFlash" && sel.Sel.Name != "setSuccessFlash") {
					return true
				}
				for _, arg := range call.Args {
					if offender := themeCopyReference(arg, vocabulary); offender != "" {
						t.Errorf("%s:%d passes the theme copy %s to %s; every theme signal is raised through setThemeFlash, which is what stamps the theme precedence tier (see flashSlotClaim)",
							name, source.Position(call.Pos()).Line, offender, sel.Sel.Name)
					}
				}
				return true
			})
		}
	})
}

func themeCopyVocabulary(files map[string]*ast.File) map[string]bool {
	vocabulary := map[string]bool{}
	for name, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.CONST && d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, ident := range value.Names {
						if i < len(value.Values) && mentionsTheme(value.Values[i]) {
							vocabulary[ident.Name] = true
						}
					}
				}
			case *ast.FuncDecl:
				if strings.HasPrefix(name, "theme") && returnsString(d) {
					vocabulary[d.Name.Name] = true
				}
			}
		}
	}
	return vocabulary
}

func mentionsTheme(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if strings.Contains(strings.ToLower(lit.Value), "theme") {
			found = true
			return false
		}
		return true
	})
	return found
}

func returnsString(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, result := range fn.Type.Results.List {
		if ident, ok := result.Type.(*ast.Ident); ok && ident.Name == "string" {
			return true
		}
	}
	return false
}

func themeCopyReference(expr ast.Expr, vocabulary map[string]bool) string {
	offender := ""
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if vocabulary[node.Name] {
				offender = node.Name
				return false
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING && strings.Contains(strings.ToLower(node.Value), "theme") {
				offender = node.Value
				return false
			}
		}
		return true
	})
	return offender
}

type themeFlashSurface struct {
	name         string
	page         page
	query        string
	band         func(Model) (noticeBandRole, string, bool)
	renderedBand func(Model) string
	slot         func(Model) string
	headerRow    func(Model) string
	view         func(Model) string
	filterState  func(Model) list.FilterState
}

func sessionsFlashSurface() themeFlashSurface {
	return themeFlashSurface{
		name:         "sessions",
		page:         PageSessions,
		query:        "alpha",
		band:         Model.activeNoticeBand,
		renderedBand: Model.renderActiveNoticeBand,
		slot:         Model.renderSessionBandSlot,
		headerRow:    func(m Model) string { return sectionHeaderRow(m.applySectionHeader(m.sessionList.View())) },
		view:         Model.viewSessionList,
		filterState:  func(m Model) list.FilterState { return m.sessionList.FilterState() },
	}
}

func projectsFlashSurface() themeFlashSurface {
	return themeFlashSurface{
		name:         "projects",
		page:         PageProjects,
		query:        "one",
		band:         Model.activeProjectNoticeBand,
		renderedBand: Model.renderActiveProjectNoticeBand,
		slot:         Model.renderProjectBandSlot,
		headerRow:    func(m Model) string { return sectionHeaderRow(m.applyProjectsSectionHeader(m.projectList.View())) },
		view:         Model.viewProjectList,
		filterState:  func(m Model) list.FilterState { return m.projectList.FilterState() },
	}
}

func themeFlashSurfaces() []themeFlashSurface {
	return []themeFlashSurface{sessionsFlashSurface(), projectsFlashSurface()}
}

var themeFlashFilterStates = []struct {
	name string
	lock bool
	want list.FilterState
}{
	{name: "the filter input focused", lock: false, want: list.Filtering},
	{name: "a filter applied", lock: true, want: list.FilterApplied},
}

// Colourless so the production `t` raises the NO_COLOR block — a theme signal
// reachable by keypress while a filter is applied.
func themeFlashModel(t *testing.T, p page) Model {
	t.Helper()
	m, _ := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
		page:       p,
		colourless: true,
		contentW:   entryContentW,
		contentH:   entryContentH,
	})
	return m
}

func applyThemeFlashFilter(t *testing.T, m Model, query string, locked bool) Model {
	t.Helper()
	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range query {
		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if locked {
		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	return m
}

// The scan stops at the section-header row: a selected list row draws its
// cursor with the same glyph in the same column further down the page.
func noticeSlotBandRows(s themeFlashSurface, m Model) int {
	lines := strings.Split(ansi.Strip(s.view(m)), "\n")
	stop := len(lines)
	if header := strings.TrimRight(ansi.Strip(s.headerRow(m)), " "); header != "" {
		if idx := lineIndexContaining(lines, header); idx >= 0 {
			stop = idx
		}
	}
	rows := 0
	for _, line := range lines[:stop] {
		if strings.HasPrefix(line, noticeBarGlyph) {
			rows++
		}
	}
	return rows
}

func requireBandOwnsTheSlot(t *testing.T, s themeFlashSurface, m Model, want string) {
	t.Helper()
	role, message, ok := s.band(m)
	if !ok {
		t.Fatalf("%s: no band was arbitrated while %q was live", s.name, want)
	}
	if role != bandWarning || message != want {
		t.Errorf("%s: the arbitrated band is (role %v, message %q), want (bandWarning, %q)", s.name, role, message, want)
	}
	view := s.view(m)
	if !strings.Contains(ansi.Strip(view), want) {
		t.Errorf("%s: the composed frame carries no %q band:\n%s", s.name, want, ansi.Strip(view))
	}
	band := s.renderedBand(m)
	if gotRows, wantRows := noticeSlotBandRows(s, m), lipgloss.Height(band); gotRows != wantRows {
		t.Errorf("%s: the notice slot carries %d `%s` band rows, want %d (exactly one arbitrated band)", s.name, gotRows, noticeBarGlyph, wantRows)
	}
	if gotRows, wantRows := lipgloss.Height(s.slot(m)), lipgloss.Height(band)+1; gotRows != wantRows {
		t.Errorf("%s: the band slot is %d rows, want %d (one band + its blank breathing row)", s.name, gotRows, wantRows)
	}
}

func requireFilterRowUnaffected(t *testing.T, s themeFlashSurface, m Model, baseline, query string) {
	t.Helper()
	got := s.headerRow(m)
	if got != baseline {
		t.Errorf("%s: the filter row changed while a flash held the band slot:\n got %q\nwant %q", s.name, got, baseline)
	}
	if !strings.Contains(ansi.Strip(got), filterPromptPrefix+query) {
		t.Errorf("%s: the filter row no longer carries %q: %q", s.name, filterPromptPrefix+query, ansi.Strip(got))
	}
}

func requireBandAboveTheFilterRow(t *testing.T, s themeFlashSurface, m Model, flash, query string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(s.view(m)), "\n")
	bandIdx := lineIndexContaining(lines, flash)
	filterIdx := lineIndexContaining(lines, filterPromptPrefix+query)
	if bandIdx < 0 || filterIdx < 0 {
		t.Fatalf("%s: missing a landmark (band=%d filter=%d):\n%s", s.name, bandIdx, filterIdx, strings.Join(lines, "\n"))
	}
	if filterIdx-bandIdx != 2 {
		t.Errorf("%s: the filter row is %d rows below the band, want 2 (band → blank → filter row)", s.name, filterIdx-bandIdx)
	}
	if blank := lines[bandIdx+1]; strings.TrimSpace(blank) != "" {
		t.Errorf("%s: the row between the band and the filter row must be blank, got %q", s.name, blank)
	}
}

func raiseBlockedThemeFlash(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := pressThemeKeyCmd(t, m)
	requireBlocked(t, m, cmd, wantNoColorEntryFlash)
	if m.flashOrigin != flashOriginTheme {
		t.Fatalf("the blocked `t` raised a flash with origin %v, want the theme origin", m.flashOrigin)
	}
	return m
}

func filteredThemeFlashModel(t *testing.T, s themeFlashSurface, locked bool, want list.FilterState) (Model, string) {
	t.Helper()
	m := applyThemeFlashFilter(t, themeFlashModel(t, s.page), s.query, locked)
	if got := s.filterState(m); got != want {
		t.Fatalf("%s: the filter state is %v, want %v", s.name, got, want)
	}
	baseline := s.headerRow(m)
	if !strings.Contains(ansi.Strip(baseline), filterPromptPrefix+s.query) {
		t.Fatalf("%s: the filter row does not carry %q, so a comparison against it says nothing: %q", s.name, filterPromptPrefix+s.query, ansi.Strip(baseline))
	}
	if _, _, ok := s.band(m); ok {
		t.Fatalf("%s: a band already owns the slot before the flash is raised", s.name)
	}
	return m, baseline
}

func TestThemeFlash_ReachesBandUnderAppliedFilterOnSessions(t *testing.T) {
	s := sessionsFlashSurface()
	m, baseline := filteredThemeFlashModel(t, s, true, list.FilterApplied)

	m = raiseBlockedThemeFlash(t, m)

	requireBandOwnsTheSlot(t, s, m, wantNoColorEntryFlash)
	requireFilterRowUnaffected(t, s, m, baseline, s.query)
	requireBandAboveTheFilterRow(t, s, m, wantNoColorEntryFlash, s.query)
	if got := s.filterState(m); got != list.FilterApplied {
		t.Errorf("the flash moved the filter state to %v, want it left applied", got)
	}
}

// Raised directly, not by keypress: `t` is a literal filter character while the
// input is focused.
func TestThemeFlash_ReachesBandUnderLiveFilterOnSessions(t *testing.T) {
	s := sessionsFlashSurface()
	m, baseline := filteredThemeFlashModel(t, s, false, list.Filtering)

	(&m).setThemeFlash(wantNoColorEntryFlash)

	requireBandOwnsTheSlot(t, s, m, wantNoColorEntryFlash)
	requireFilterRowUnaffected(t, s, m, baseline, s.query)
	requireBandAboveTheFilterRow(t, s, m, wantNoColorEntryFlash, s.query)
	if got := s.filterState(m); got != list.Filtering {
		t.Errorf("the flash moved the filter state to %v, want the input left focused", got)
	}
}

func TestThemeFlash_ReachesBandUnderAppliedFilterOnProjects(t *testing.T) {
	s := projectsFlashSurface()

	for _, tc := range themeFlashFilterStates {
		t.Run(tc.name, func(t *testing.T) {
			m, baseline := filteredThemeFlashModel(t, s, tc.lock, tc.want)

			if tc.lock {
				m = raiseBlockedThemeFlash(t, m)
			} else {
				(&m).setThemeFlash(wantNoColorEntryFlash)
			}

			requireBandOwnsTheSlot(t, s, m, wantNoColorEntryFlash)
			requireFilterRowUnaffected(t, s, m, baseline, s.query)
			requireBandAboveTheFilterRow(t, s, m, wantNoColorEntryFlash, s.query)
			if got := s.filterState(m); got != tc.want {
				t.Errorf("the flash moved the filter state to %v, want %v", got, tc.want)
			}
		})
	}
}

// The filter line never contends for the band slot, so a theme flash reaches the
// band whatever the filter is doing — the reason the required behaviour holds.
func TestThemeFlash_FilterLineIsNotABandContender(t *testing.T) {
	for _, s := range themeFlashSurfaces() {
		t.Run(s.name+"/an applied filter claims no band slot", func(t *testing.T) {
			m, baseline := filteredThemeFlashModel(t, s, true, list.FilterApplied)

			if role, message, ok := s.band(m); ok {
				t.Errorf("%s: an applied filter arbitrated a band (role %v, message %q); the filter line is not a band contender", s.name, role, message)
			}
			if got := s.slot(m); got != "" {
				t.Errorf("%s: the band slot renders %q under an applied filter, want nothing", s.name, got)
			}
			if !strings.Contains(ansi.Strip(baseline), filterPromptPrefix+s.query) {
				t.Errorf("%s: the filter line is not on the section-header row: %q", s.name, ansi.Strip(baseline))
			}
		})

		t.Run(s.name+"/a theme flash and an applied filter occupy separate rows", func(t *testing.T) {
			m, baseline := filteredThemeFlashModel(t, s, true, list.FilterApplied)

			m = raiseBlockedThemeFlash(t, m)

			slot := ansi.Strip(s.slot(m))
			if !strings.Contains(slot, wantNoColorEntryFlash) {
				t.Errorf("%s: the band slot carries no theme flash: %q", s.name, slot)
			}
			if strings.Contains(slot, filterPromptPrefix+s.query) {
				t.Errorf("%s: the band slot carries the filter line %q, want it on the section-header row alone: %q", s.name, filterPromptPrefix+s.query, slot)
			}
			if got := s.headerRow(m); got != baseline {
				t.Errorf("%s: the section-header row changed when the theme flash took the band slot:\n got %q\nwant %q", s.name, got, baseline)
			}
		})

		// The tier is a forward guard, not the mechanism: with one flashText its two
		// arms return the same claim, so it changes no rendered outcome today.
		t.Run(s.name+"/the theme tier changes no outcome under a filter", func(t *testing.T) {
			const message = "__SAME_UNDER_FILTER__"

			themed, _ := filteredThemeFlashModel(t, s, true, list.FilterApplied)
			(&themed).setThemeFlash(message)

			ordinary, _ := filteredThemeFlashModel(t, s, true, list.FilterApplied)
			ordinary.setFlash(message)

			if got, want := s.slot(themed), s.slot(ordinary); got != want {
				t.Errorf("%s: the theme tier changed the band slot under a filter:\n got %q\nwant %q", s.name, got, want)
			}
			if got, want := s.headerRow(themed), s.headerRow(ordinary); got != want {
				t.Errorf("%s: the theme tier changed the section-header row under a filter:\n got %q\nwant %q", s.name, got, want)
			}
		})
	}
}

func TestThemeFlash_NonThemeFlashUnchanged(t *testing.T) {
	const ordinary = "__ORDINARY_FLASH__"

	for _, s := range themeFlashSurfaces() {
		for _, tc := range themeFlashFilterStates {
			t.Run(s.name+"/"+tc.name, func(t *testing.T) {
				m, baseline := filteredThemeFlashModel(t, s, tc.lock, tc.want)

				m.setFlash(ordinary)

				if m.flashOrigin != flashOriginDefault {
					t.Errorf("an ordinary flash carries origin %v, want the default", m.flashOrigin)
				}
				requireBandOwnsTheSlot(t, s, m, ordinary)
				requireFilterRowUnaffected(t, s, m, baseline, s.query)
				want := renderNoticeBand(bandWarning, ordinary, noticeBandOnBandText(bandWarning, m.themeState.active), m.contentWidth(), m.themeState.active, m.colourless)
				if got := s.renderedBand(m); got != want {
					t.Errorf("%s: an ordinary flash's band differs from the shared primitive's render:\n got %q\nwant %q", s.name, got, want)
				}
			})
		}
	}

	t.Run("it still outranks the persistent bands beneath it", func(t *testing.T) {
		sessions := noticeBandModel("alpha-row")
		sessions.byTagSignpost = true
		sessions.setFlash(ordinary)
		if role, message, ok := sessions.activeNoticeBand(); !ok || role != bandWarning || message != ordinary {
			t.Errorf("Sessions arbiter = (role %v, message %q, ok %v), want the ordinary flash over the signpost", role, message, ok)
		}

		projects := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})
		projects.setFlash(ordinary)
		if role, message, ok := projects.activeProjectNoticeBand(); !ok || role != bandWarning || message != ordinary {
			t.Errorf("Projects arbiter = (role %v, message %q, ok %v), want the ordinary flash over the command banner", role, message, ok)
		}
	})

	t.Run("its lifecycle is the shared one", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		m.setFlash(ordinary)

		superseded, _ := m.Update(flashTickMsg{Gen: m.flashGen - 1})
		if got := superseded.(Model).flashText; got != ordinary {
			t.Errorf("a superseded tick left the ordinary flash %q, want it standing", got)
		}
		cleared, _ := m.Update(flashTickMsg{Gen: m.flashGen})
		if got := cleared.(Model).flashText; got != "" {
			t.Errorf("the matching tick left the ordinary flash %q, want it cleared", got)
		}
	})
}

func TestThemeFlash_SingleSlotHolds(t *testing.T) {
	for _, s := range themeFlashSurfaces() {
		for _, tc := range themeFlashFilterStates {
			t.Run(s.name+"/"+tc.name, func(t *testing.T) {
				m, _ := filteredThemeFlashModel(t, s, tc.lock, tc.want)

				(&m).setThemeFlash(wantNoColorEntryFlash)

				if got := lipgloss.Height(s.renderedBand(m)); got != 1 {
					t.Errorf("%s: the arbitrated band is %d rows, want 1 at this width", s.name, got)
				}
				requireBandOwnsTheSlot(t, s, m, wantNoColorEntryFlash)

				(&m).clearFlash()
				if got := noticeSlotBandRows(s, m); got != 0 {
					t.Errorf("%s: the notice slot still carries %d band rows after the clear, want 0", s.name, got)
				}
			})
		}
	}

	t.Run("a pending burst swallows t, so the Opening band is never due with one", func(t *testing.T) {
		s := sessionsFlashSurface()
		m := themeFlashModel(t, s.page)
		m.burstPending = true
		m.burstDone, m.burstTotal = 1, 3

		m, cmd := pressThemeKeyCmd(t, m)

		if cmd != nil {
			t.Error("`t` returned a command while the burst input-lock was engaged")
		}
		if got := m.flashText; got != "" {
			t.Errorf("`t` raised %q during a pending burst, want it swallowed", got)
		}
		if got := noticeSlotBandRows(s, m); got != 0 {
			t.Errorf("the notice slot carries %d band rows during a pending burst, want 0", got)
		}
		if header := ansi.Strip(s.headerRow(m)); !strings.Contains(header, "Opening") {
			t.Errorf("the section-header row is %q, want the Opening band to own it", header)
		}
	})
}

func TestThemeFlash_ComposesWithMultiSelect(t *testing.T) {
	s := sessionsFlashSurface()
	m := themeFlashModel(t, s.page)
	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.MultiSelectActive() {
		t.Fatal("precondition: `m` did not enter multi-select")
	}
	marked := m.SelectedSessionCount()
	if marked == 0 {
		t.Fatal("precondition: entering multi-select marked nothing, so an intact set proves nothing")
	}

	m = raiseBlockedThemeFlash(t, m)

	role, message, ok := m.activeNoticeBand()
	if !ok || role != bandWarning || message != wantNoColorEntryFlash {
		t.Errorf("the arbitrated band is (role %v, message %q, ok %v), want the theme flash over the multi-select banner", role, message, ok)
	}
	if !m.MultiSelectActive() || m.SelectedSessionCount() != marked {
		t.Errorf("the flash disturbed the mode: active=%v marked=%d, want active with %d marked", m.MultiSelectActive(), m.SelectedSessionCount(), marked)
	}
	lines := strings.Split(ansi.Strip(m.viewSessionList()), "\n")
	bandIdx := lineIndexContaining(lines, wantNoColorEntryFlash)
	bannerIdx := lineIndexContaining(lines, "selected")
	if bandIdx < 0 || bannerIdx < 0 {
		t.Fatalf("missing a landmark (band=%d banner=%d):\n%s", bandIdx, bannerIdx, strings.Join(lines, "\n"))
	}
	if bannerIdx <= bandIdx {
		t.Errorf("the multi-select banner is at row %d and the band at row %d; the band renders above it", bannerIdx, bandIdx)
	}
	if got := noticeSlotBandRows(s, m); got != 1 {
		t.Errorf("the notice slot carries %d band rows alongside the banner, want exactly 1", got)
	}
}

func TestThemeFlash_LifecycleUntouched(t *testing.T) {
	for _, s := range themeFlashSurfaces() {
		raise := func(t *testing.T) Model {
			t.Helper()
			return raiseBlockedThemeFlash(t, themeFlashModel(t, s.page))
		}

		t.Run(s.name+"/the matching tick clears it", func(t *testing.T) {
			m := raise(t)

			updated, _ := m.Update(flashTickMsg{Gen: m.flashGen})

			if got := updated.(Model).flashText; got != "" {
				t.Errorf("the matching tick left the theme flash %q, want it cleared", got)
			}
		})

		t.Run(s.name+"/a superseded tick is dropped", func(t *testing.T) {
			m := raise(t)

			updated, _ := m.Update(flashTickMsg{Gen: m.flashGen - 1})

			if got := updated.(Model).flashText; got != wantNoColorEntryFlash {
				t.Errorf("a superseded tick left the theme flash %q, want it standing", got)
			}
		})

		t.Run(s.name+"/the next actionable key clears it", func(t *testing.T) {
			m := raise(t)

			m = pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})

			if got := m.flashText; got != "" {
				t.Errorf("the next actionable key left the theme flash %q, want it cleared", got)
			}
		})
	}
}

func TestThemeFlash_NoNewBandRole(t *testing.T) {
	const message = "__SAME_BAND__"

	for _, s := range themeFlashSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			ordinary := themeFlashModel(t, s.page)
			ordinary.setFlash(message)

			themed := themeFlashModel(t, s.page)
			(&themed).setThemeFlash(message)

			if got, want := s.renderedBand(themed), s.renderedBand(ordinary); got != want {
				t.Errorf("%s: a theme flash's band differs from an ordinary flash's for the same message:\n got %q\nwant %q", s.name, got, want)
			}
			if got, want := s.slot(themed), s.slot(ordinary); got != want {
				t.Errorf("%s: a theme flash's slot differs from an ordinary flash's:\n got %q\nwant %q", s.name, got, want)
			}
			role, _, ok := s.band(themed)
			if !ok || role != bandWarning {
				t.Errorf("%s: a theme flash arbitrates to (role %v, ok %v), want the existing bandWarning role", s.name, role, ok)
			}

			(&themed).clearFlash()
			if _, _, ok := s.band(themed); ok {
				t.Errorf("%s: a contender still owns the slot after the theme flash cleared", s.name)
			}
			if got := s.slot(themed); got != "" {
				t.Errorf("%s: the band slot renders %q after the theme flash cleared, want nothing — no permanent entry is added", s.name, got)
			}
		})
	}
}
