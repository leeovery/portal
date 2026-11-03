package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
)

const (
	wantSessionsFooterCluster = "⏎ attach · / filter · ␣ preview · s switch view · x projects · t theme · m multi"
	wantProjectsFooterCluster = "⏎ new session · x sessions · e edit · / filter · t theme"
	wantFooterHelpAnchor      = "? help"
)

func footerRowVisible(t *testing.T, footer string) string {
	t.Helper()
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("a condensed footer must be 2 rows (rule + key row), got %d:\n%s", len(lines), footer)
	}
	return footerVisible(lines[1])
}

func splitFooterRow(row string) (cluster, anchor string) {
	if trimmed := strings.TrimRight(row, " "); strings.HasSuffix(trimmed, wantFooterHelpAnchor) {
		return strings.TrimRight(strings.TrimSuffix(trimmed, wantFooterHelpAnchor), " "), wantFooterHelpAnchor
	}
	return strings.TrimRight(row, " "), ""
}

func TestFooterRevision_SessionsPinnedCopy(t *testing.T) {
	row := footerRowVisible(t, renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false))
	cluster, anchor := splitFooterRow(row)

	if cluster != wantSessionsFooterCluster {
		t.Errorf("Sessions footer cluster:\n got  %q\n want %q", cluster, wantSessionsFooterCluster)
	}
	if anchor != wantFooterHelpAnchor {
		t.Errorf("Sessions footer anchor = %q, want the right-aligned %q", anchor, wantFooterHelpAnchor)
	}
}

func TestFooterRevision_ProjectsPinnedCopy(t *testing.T) {
	row := footerRowVisible(t, renderProjectsFooter(projectsKeymap(), referenceFooterWidth, testDarkTheme(t), false))
	cluster, anchor := splitFooterRow(row)

	if cluster != wantProjectsFooterCluster {
		t.Errorf("Projects footer cluster:\n got  %q\n want %q", cluster, wantProjectsFooterCluster)
	}
	if anchor != wantFooterHelpAnchor {
		t.Errorf("Projects footer anchor = %q, want the right-aligned %q", anchor, wantFooterHelpAnchor)
	}
}

func TestFooterRevision_NavigateIsNonCore(t *testing.T) {
	th := testDarkTheme(t)
	for _, tc := range []struct {
		page    string
		footer  string
		help    string
		entries []keymapEntry
	}{
		{
			page:    "sessions",
			footer:  footerRowVisible(t, renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, th, false)),
			help:    footerVisible(helpModalBody(sessionsKeymap(), th, false)),
			entries: sessionsKeymap(),
		},
		{
			page:    "projects",
			footer:  footerRowVisible(t, renderProjectsFooter(projectsKeymap(), referenceFooterWidth, th, false)),
			help:    footerVisible(helpModalBody(projectsKeymap(), th, false)),
			entries: projectsKeymap(),
		},
	} {
		t.Run(tc.page, func(t *testing.T) {
			if strings.Contains(tc.footer, "navigate") {
				t.Errorf("the %s footer still lists `↑↓ navigate` (the footer drops it):\n%s", tc.page, tc.footer)
			}
			if !strings.Contains(tc.help, "Move selection") {
				t.Errorf("the %s help body must still list the nav row (help keeps it):\n%s", tc.page, tc.help)
			}
			for _, e := range tc.entries {
				if e.Key == "↑↓" && e.Core {
					t.Errorf("the %s nav entry is still Core; it must be non-core (listed, not footed)", tc.page)
				}
			}
		})
	}
}

func TestFooterRevision_MultiLabelIsShort(t *testing.T) {
	th := testDarkTheme(t)
	row := footerRowVisible(t, renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, th, false))

	if !strings.Contains(row, "m multi") {
		t.Errorf("the Sessions footer must carry `m multi`:\n%s", row)
	}
	if strings.Contains(row, "multi-select") {
		t.Errorf("the Sessions footer must NOT carry the long `m multi-select` label:\n%s", row)
	}
	if body := footerVisible(helpModalBody(sessionsKeymap(), th, false)); !strings.Contains(body, "Multi-select mode") {
		t.Errorf("the help body keeps the long `Multi-select mode` label (only the FOOTER is shortened):\n%s", body)
	}
}

func footerRevisionSessionsModel(t *testing.T, colourless bool) Model {
	t.Helper()
	m := NewModelWithSessions([]Session{
		{Name: "alpha", Answered: true},
		{Name: "bravo", Answered: true},
	})
	m.termWidth = referenceFooterWidth + 2*Hinset
	m.termHeight = 30 + 2*Vinset
	m.colourless = colourless
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	if got := m.contentWidth(); got != referenceFooterWidth {
		t.Fatalf("fixture: content width is %d, want the reference %d", got, referenceFooterWidth)
	}
	return m
}

func footerRevisionProjectsModel(t *testing.T, colourless bool) Model {
	t.Helper()
	m := footerRevisionSessionsModel(t, colourless)
	projects := []project.Project{{Path: "/p/one", Name: "one"}, {Path: "/p/two", Name: "two"}}
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	m.activePage = PageProjects
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	return m
}

func keymapHasKey(entries []keymapEntry, key string) bool {
	for _, e := range entries {
		if e.Key == key {
			return true
		}
	}
	return false
}

func TestFooterRevision_BlockedThemeKeyFilteredInLockstep(t *testing.T) {
	helpBodyThroughView := func(view func(m Model) string, m Model) string {
		m.modal = modalHelp
		return footerVisible(view(m))
	}

	for _, tc := range []struct {
		page     string
		model    func(t *testing.T, colourless bool) Model
		keymap   func(m Model) []keymapEntry
		footer   func(m Model) string
		view     func(m Model) string
		clusters string
	}{
		{
			page:     "sessions",
			model:    footerRevisionSessionsModel,
			keymap:   Model.sessionsHelpKeymap,
			footer:   Model.renderSessionsFooterForFilterState,
			view:     Model.viewSessionList,
			clusters: wantSessionsFooterCluster,
		},
		{
			page:     "projects",
			model:    footerRevisionProjectsModel,
			keymap:   Model.projectsHelpKeymap,
			footer:   Model.renderProjectsFooterForFilterState,
			view:     Model.viewProjectList,
			clusters: wantProjectsFooterCluster,
		},
	} {
		t.Run(tc.page, func(t *testing.T) {
			live := tc.model(t, false)
			if !keymapHasKey(tc.keymap(live), "t") {
				t.Errorf("the %s call-site keymap must LIST t when the panel is available", tc.page)
			}
			liveRow := footerRowVisible(t, tc.footer(live))
			if cluster, _ := splitFooterRow(liveRow); cluster != tc.clusters {
				t.Errorf("the unblocked %s footer cluster:\n got  %q\n want %q", tc.page, cluster, tc.clusters)
			}
			if body := helpBodyThroughView(tc.view, live); !strings.Contains(body, "Theme picker") {
				t.Errorf("the unblocked %s help modal must list the Theme picker row:\n%s", tc.page, body)
			}

			blocked := tc.model(t, true)
			if keymapHasKey(tc.keymap(blocked), "t") {
				t.Errorf("the %s call-site keymap must DROP t under NO_COLOR", tc.page)
			}
			blockedRow := footerRowVisible(t, tc.footer(blocked))
			if strings.Contains(blockedRow, "t theme") {
				t.Errorf("the blocked %s footer still advertises `t theme` (footer and help filter in lockstep):\n%s", tc.page, blockedRow)
			}
			if body := helpBodyThroughView(tc.view, blocked); strings.Contains(body, "Theme picker") {
				t.Errorf("the blocked %s help modal still lists the Theme picker row (its view passes the UNFILTERED descriptor):\n%s", tc.page, body)
			}
		})
	}
}

func TestFooterRevision_BlockedMultiKeyFilteredInLockstep(t *testing.T) {
	th := testDarkTheme(t)

	supported := unsupportedResolvedModel(t, ghosttyIdentity())
	if !keymapHasKey(supported.sessionsHelpKeymap(), "m") {
		t.Fatal("precondition: a supported terminal must keep the m entry")
	}
	supportedRow := footerRowVisible(t, renderSessionsFooter(supported.sessionsHelpKeymap(), referenceFooterWidth, th, false))
	if !strings.Contains(supportedRow, "m multi") {
		t.Errorf("a supported terminal's footer must carry `m multi`:\n%s", supportedRow)
	}

	blocked := unsupportedResolvedModel(t, appleTerminalIdentity())
	if !blocked.DetectUnsupported() || blocked.multiSelectMode {
		t.Fatal("precondition: the fixture must resolve unsupported and not be in multi-select")
	}
	if keymapHasKey(blocked.sessionsHelpKeymap(), "m") {
		t.Error("the call-site keymap must DROP m on a resolved-unsupported terminal")
	}
	blockedRow := footerRowVisible(t, renderSessionsFooter(blocked.sessionsHelpKeymap(), referenceFooterWidth, th, false))
	if strings.Contains(blockedRow, "m multi") {
		t.Errorf("the blocked footer still advertises `m multi` (footer and help filter in lockstep):\n%s", blockedRow)
	}
	if body := footerVisible(helpModalBody(blocked.sessionsHelpKeymap(), th, false)); strings.Contains(body, "Multi-select mode") {
		t.Errorf("the blocked help body still lists the multi-select row:\n%s", body)
	}
}

func TestFooterRevision_BudgetMatchesFilteredRender(t *testing.T) {
	for _, tc := range []struct {
		page       string
		model      func(t *testing.T, colourless bool) Model
		filtered   func(m Model) string
		unfiltered func(m Model) string
		composed   func(m Model) string
		budget     func(m Model) int
	}{
		{
			page:  "sessions",
			model: footerRevisionSessionsModel,
			filtered: func(m Model) string {
				return renderSessionsFooter(m.sessionsHelpKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
			},
			unfiltered: func(m Model) string {
				return renderSessionsFooter(sessionsKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
			},
			composed: Model.renderSessionsFooterForFilterState,
			budget:   func(m Model) int { return m.sessionFooterHeight(m.contentWidth()) },
		},
		{
			page:  "projects",
			model: footerRevisionProjectsModel,
			filtered: func(m Model) string {
				return renderProjectsFooter(m.projectsHelpKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
			},
			unfiltered: func(m Model) string {
				return renderProjectsFooter(projectsKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
			},
			composed: Model.renderProjectsFooterForFilterState,
			budget:   func(m Model) int { return m.projectFooterHeight(m.contentWidth()) },
		},
	} {
		t.Run(tc.page, func(t *testing.T) {
			m := tc.model(t, true)

			filtered := tc.filtered(m)
			if filtered == tc.unfiltered(m) {
				t.Fatal("precondition: the blocked footer must differ from the unfiltered one")
			}

			if got := tc.composed(m); got != filtered {
				t.Errorf("the composed %s footer is not the filtered render:\n got=%q\nwant=%q", tc.page, got, filtered)
			}
			if got, want := tc.budget(m), lipgloss.Height(filtered); got != want {
				t.Errorf("the %s footer budget reserves %d rows, the filtered render is %d", tc.page, got, want)
			}
		})
	}
}

func TestFooterRevision_StaticDescriptorsUnfiltered(t *testing.T) {
	wantSessions, wantProjects := sessionsKeymap(), projectsKeymap()

	assertStatic := func(t *testing.T, state string) {
		t.Helper()
		if got := sessionsKeymap(); !reflect.DeepEqual(got, wantSessions) {
			t.Errorf("[%s] the static Sessions descriptor changed:\ngot  %+v\nwant %+v", state, got, wantSessions)
		}
		if got := projectsKeymap(); !reflect.DeepEqual(got, wantProjects) {
			t.Errorf("[%s] the static Projects descriptor changed:\ngot  %+v\nwant %+v", state, got, wantProjects)
		}
		for _, key := range []string{"t", "m"} {
			if !keymapHasKey(sessionsKeymap(), key) {
				t.Errorf("[%s] the static Sessions descriptor must always carry %q", state, key)
			}
		}
		if !keymapHasKey(projectsKeymap(), "t") {
			t.Errorf("[%s] the static Projects descriptor must always carry t", state)
		}
	}

	blockedTheme := footerRevisionProjectsModel(t, true)
	if keymapHasKey(blockedTheme.sessionsHelpKeymap(), "t") || keymapHasKey(blockedTheme.projectsHelpKeymap(), "t") {
		t.Fatal("precondition: NO_COLOR must strip t from both call-site keymaps")
	}
	assertStatic(t, "theme blocked")

	blockedMulti := unsupportedResolvedModel(t, appleTerminalIdentity())
	if keymapHasKey(blockedMulti.sessionsHelpKeymap(), "m") {
		t.Fatal("precondition: an unsupported terminal must strip m from the Sessions call-site keymap")
	}
	assertStatic(t, "multi blocked")
}

func helpAnchorWidth(t *testing.T, th theme.Theme) int {
	t.Helper()
	_, anchor := splitFooterEntries(sessionsKeymap())
	if anchor == nil {
		t.Fatal("the Sessions descriptor must carry the right-aligned ? help anchor")
	}
	return lipgloss.Width(renderFooterEntry(*anchor, th.AccentPrimary, th, false))
}

func allowedFooterClusters(entries []keymapEntry) []string {
	core, _ := splitFooterEntries(entries)
	parts := make([]string, 0, len(core))
	for _, e := range core {
		parts = append(parts, e.Key+footerKeyLabelGap+e.Action)
	}
	allowed := []string{"", footerEllipsis, strings.Join(parts, footerEntrySeparator)}
	for n := 1; n < len(parts); n++ {
		allowed = append(allowed, strings.Join(parts[:n], footerEntrySeparator)+footerEntrySeparator+footerEllipsis)
	}
	return allowed
}

func footerClusterEntryCount(cluster string) int {
	trimmed := strings.TrimSuffix(cluster, footerEntrySeparator+footerEllipsis)
	if trimmed == "" || trimmed == footerEllipsis {
		return 0
	}
	return len(strings.Split(trimmed, footerEntrySeparator))
}

func TestFooterRevision_HelpAnchorSurvivesNarrowing(t *testing.T) {
	th := testDarkTheme(t)
	anchorW := helpAnchorWidth(t, th)
	allowed := allowedFooterClusters(sessionsKeymap())

	previous := -1
	for w := 120; w >= anchorW; w-- {
		row := footerRowVisible(t, renderSessionsFooter(sessionsKeymap(), w, th, false))
		cluster, anchor := splitFooterRow(row)

		if anchor != wantFooterHelpAnchor {
			t.Fatalf("at width %d the ? help anchor was dropped; it is never dropped while it fits:\n%q", w, row)
		}
		if !slices.Contains(allowed, cluster) {
			t.Fatalf("at width %d the cluster %q is not a legal degrade step (wrapped, truncated, or dropped from the wrong end)", w, cluster)
		}
		count := footerClusterEntryCount(cluster)
		if previous >= 0 {
			if count > previous {
				t.Fatalf("at width %d the cluster grew back to %d entries from %d — the drop must be monotonic as the row narrows", w, count, previous)
			}
			if previous-count > 1 {
				t.Fatalf("at width %d the cluster lost %d entries in one cell (from %d to %d) — the cluster drops one at a time", w, previous-count, previous, count)
			}
		}
		previous = count
	}
	if previous != 0 {
		t.Errorf("at the anchor's own width the cluster still carries %d entries, want none", previous)
	}
}

// The ladders assert legality and monotonicity but no absolute width, and the
// first degrade step is the boundary an unfitted cluster falls off whole: with
// the anchor reserved the full cluster survives at cluster+1+anchor and gives up
// exactly one entry a cell below it.
func TestFooterRevision_FirstDegradeWidthIsPinned(t *testing.T) {
	th := testDarkTheme(t)
	anchorW := helpAnchorWidth(t, th)

	sessionsCore, _ := splitFooterEntries(sessionsKeymap())
	filtering := filteringFooterEntries(th)

	for _, tc := range []struct {
		name    string
		cluster string
		count   int
		render  func(w int) string
	}{
		{
			name:    "sessions",
			cluster: footerVisible(renderFooterCluster(sessionsCore, th, false)),
			count:   len(sessionsCore),
			render:  func(w int) string { return renderSessionsFooter(sessionsKeymap(), w, th, false) },
		},
		{
			name:    "filtering",
			cluster: footerVisible(renderFilterCluster(filtering, th, false)),
			count:   len(filtering),
			render:  func(w int) string { return renderFilteringFooter(w, th, false) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			boundary := lipgloss.Width(tc.cluster) + 1 + anchorW

			cluster, anchor := splitFooterRow(footerRowVisible(t, tc.render(boundary)))
			if cluster != tc.cluster {
				t.Errorf("at the boundary width %d the cluster:\n got  %q\n want the full %q", boundary, cluster, tc.cluster)
			}
			if anchor != wantFooterHelpAnchor {
				t.Errorf("at the boundary width %d the ? help anchor was dropped", boundary)
			}

			narrow, narrowAnchor := splitFooterRow(footerRowVisible(t, tc.render(boundary-1)))
			if got, want := footerClusterEntryCount(narrow), tc.count-1; got != want {
				t.Errorf("one cell below the boundary (width %d) the cluster carries %d entries, want %d — it degrades one at a time, never whole:\n%q", boundary-1, got, want, narrow)
			}
			if narrowAnchor != wantFooterHelpAnchor {
				t.Errorf("at width %d the ? help anchor was dropped; the cluster gives way beneath it", boundary-1)
			}
		})
	}
}

func TestFooterRevision_ExtremeNarrowLadder(t *testing.T) {
	th := testDarkTheme(t)
	anchorW := helpAnchorWidth(t, th)

	t.Run("anchor alone", func(t *testing.T) {
		footer := renderSessionsFooter(sessionsKeymap(), anchorW, th, false)
		row := footerRowVisible(t, footer)
		if got := strings.TrimSpace(row); got != wantFooterHelpAnchor {
			t.Errorf("at width %d the row = %q, want the bare %q", anchorW, got, wantFooterHelpAnchor)
		}
		if got := lipgloss.Width(strings.Split(footer, "\n")[1]); got != anchorW {
			t.Errorf("at width %d the row is %d cells wide, want exactly %d", anchorW, got, anchorW)
		}
	})

	t.Run("empty below the anchor", func(t *testing.T) {
		w := anchorW - 1
		footer := renderSessionsFooter(sessionsKeymap(), w, th, false)
		row := footerRowVisible(t, footer)
		if got := strings.TrimSpace(row); got != "" {
			t.Errorf("at width %d the row = %q, want an empty row (below the anchor's width)", w, got)
		}
		if got := lipgloss.Width(strings.Split(footer, "\n")[1]); got != w {
			t.Errorf("at width %d the empty row is %d cells wide, want exactly %d", w, got, w)
		}
	})
}

func TestFooterRevision_LabelsAreNeverTruncated(t *testing.T) {
	th := testDarkTheme(t)
	for _, tc := range []struct {
		page    string
		entries []keymapEntry
		render  func(entries []keymapEntry, w int) string
	}{
		{page: "sessions", entries: sessionsKeymap(), render: func(e []keymapEntry, w int) string {
			return renderSessionsFooter(e, w, th, false)
		}},
		{page: "projects", entries: projectsKeymap(), render: func(e []keymapEntry, w int) string {
			return renderProjectsFooter(e, w, th, false)
		}},
	} {
		t.Run(tc.page, func(t *testing.T) {
			allowed := allowedFooterClusters(tc.entries)
			for w := 1; w <= 120; w++ {
				footer := tc.render(tc.entries, w)
				lines := strings.Split(footer, "\n")
				if len(lines) != 2 {
					t.Fatalf("at width %d the %s footer has %d rows, want 2 (no wrap):\n%s", w, tc.page, len(lines), footer)
				}
				for i, line := range lines {
					if lw := lipgloss.Width(line); lw != w {
						t.Errorf("at width %d the %s footer line %d is %d cells wide, want exactly %d", w, tc.page, i, lw, w)
					}
				}
				cluster, _ := splitFooterRow(footerVisible(lines[1]))
				if !slices.Contains(allowed, cluster) {
					t.Errorf("at width %d the %s cluster %q truncates a label or drops from the wrong end", w, tc.page, cluster)
				}
			}
		})
	}
}
