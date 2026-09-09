package capture_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

func TestFixtureByName(t *testing.T) {
	t.Run("unknown fixture is reported as an error", func(t *testing.T) {
		if _, err := capture.FixtureByName("does-not-exist"); err == nil {
			t.Fatal("FixtureByName(unknown) returned nil error, want error")
		}
	})

	t.Run("sessions-flat carries the deterministic Paper-mock session set", func(t *testing.T) {
		fx, err := capture.FixtureByName("sessions-flat")
		if err != nil {
			t.Fatalf("FixtureByName(sessions-flat): %v", err)
		}

		sessions, err := fx.Lister.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}

		type want struct {
			name     string
			windows  int
			attached bool
		}
		wants := []want{
			{"agentic-workflows-code-based", 3, true},
			{"agentic-workflows-codify", 2, false},
			{"fab-flowx-explore", 1, false},
			{"evvi webhooks and watchers", 4, false},
			{"aviva-proxy-qNyfEO", 1, false},
			{"designlab-web-r8suyU", 2, false},
			{"evvi-sync-engine", 1, false},
			{"fab-aws-migration", 5, false},
			{"flow-v1-api-XkkhTN", 1, false},
			{"flowx-7UKPZH", 2, false},
			{"fabric-lk26UG", 1, false},
			{"folio-Jiz4el", 1, false},
		}

		if len(sessions) != len(wants) {
			t.Fatalf("ListSessions returned %d sessions, want %d", len(sessions), len(wants))
		}
		for i, w := range wants {
			got := sessions[i]
			if got.Name != w.name {
				t.Errorf("session[%d].Name = %q, want %q", i, got.Name, w.name)
			}
			if got.Windows != w.windows {
				t.Errorf("session[%d].Windows = %d, want %d (%s)", i, got.Windows, w.windows, w.name)
			}
			if got.Attached != w.attached {
				t.Errorf("session[%d].Attached = %t, want %t (%s)", i, got.Attached, w.attached, w.name)
			}
		}
	})

	t.Run("fixture deps build a real model via the shared tui.Build constructor", func(t *testing.T) {
		fx, err := capture.FixtureByName("sessions-flat")
		if err != nil {
			t.Fatalf("FixtureByName(sessions-flat): %v", err)
		}

		m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})
}

func TestSessionsEmptyFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-empty")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-empty): %v", err)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions-empty must have ZERO sessions, got %d (the empty state would not render)", len(sessions))
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

func TestFixtureNamesIncludesSessionsEmpty(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-empty" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-empty", capture.FixtureNames())
	}
}

func TestLoadingScreenFixture(t *testing.T) {
	fx, err := capture.FixtureByName("loading-screen")
	if err != nil {
		t.Fatalf("FixtureByName(loading-screen): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading (the loading-screen fixture must park on the loading page)", m.ActivePage())
	}

	if !m.ServerStarted() {
		t.Error("loading-screen fixture must set ServerStarted so the cold-boot loading page shows")
	}

	found := false
	for _, n := range capture.FixtureNames() {
		if n == "loading-screen" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include loading-screen", capture.FixtureNames())
	}
}

func TestLoadingErrorFixture(t *testing.T) {
	fx, err := capture.FixtureByName("loading-error")
	if err != nil {
		t.Fatalf("FixtureByName(loading-error): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading (the loading-error fixture must park on the loading page)", m.ActivePage())
	}

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 2})
	model, _ = model.Update(tui.BootstrapFatalMsg{
		FailedStep: 3,
		Message:    "Portal failed to set @portal-restoring marker: permission denied",
		Err:        errors.New("permission denied"),
	})

	got := model.(tui.Model)
	if got.ActivePage() != tui.PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading (fatal must never transition to the picker)", got.ActivePage())
	}
	if got.FatalError() == nil {
		t.Error("FatalError() is nil after the seeded fatal; the error state was not entered")
	}

	visible := ansi.Strip(got.View().Content)
	if !strings.Contains(visible, "✗") {
		t.Errorf("loading-error frame missing the ✗ failure glyph:\n%s", visible)
	}
	if !strings.Contains(visible, "Portal failed to set @portal-restoring marker") {
		t.Errorf("loading-error frame missing the one-line fatal message:\n%s", visible)
	}
	if !strings.Contains(visible, "quit") {
		t.Errorf("loading-error frame missing the quit hint:\n%s", visible)
	}

	found := false
	for _, n := range capture.FixtureNames() {
		if n == "loading-error" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include loading-error", capture.FixtureNames())
	}
}

func TestSessionsByProjectFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-project")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-project): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions — by project"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Project mode)", got, want)
	}

	projects, err := fx.Deps(darkBuiltinTheme(t)).ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	if len(projects) < 2 {
		t.Errorf("sessions-by-project fixture has %d projects, want >=2 (multiple group headings)", len(projects))
	}

	idx := project.NewIndex(projects)
	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	unknown := 0
	for _, s := range sessions {
		if _, _, ok := idx.Match(s.Dir); !ok {
			unknown++
		}
	}
	if unknown == 0 {
		t.Error("sessions-by-project fixture has no Unknown catch-all member; the capture would not exercise the catch-all heading")
	}
}

func TestFixtureNamesIncludesByProject(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-by-project" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-by-project", capture.FixtureNames())
	}
}

func TestSessionsByTagFixtureExercisesMultiTagAndUntagged(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-tag")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-tag): %v", err)
	}
	projects, err := fx.Deps(darkBuiltinTheme(t)).ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}

	multiTag := false
	tagged := map[string]bool{}
	for _, p := range projects {
		if len(p.Tags) >= 2 {
			multiTag = true
		}
		if len(p.Tags) > 0 {
			tagged[p.Path] = true
		}
	}
	if !multiTag {
		t.Error("sessions-by-tag fixture has no multi-tag project; the Pattern B repeat (a session under each of its tags) is not exercised")
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	untagged := false
	for _, s := range sessions {
		if !tagged[s.Dir] {
			untagged = true
			break
		}
	}
	if !untagged {
		t.Error("sessions-by-tag fixture has no untagged-directory session; the Untagged catch-all heading is not exercised")
	}
}

func TestSessionsByTagFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-tag")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-tag): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Tag mode)", got, want)
	}

	projects, err := fx.Deps(darkBuiltinTheme(t)).ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	tagged := false
	for _, p := range projects {
		if len(p.Tags) > 0 {
			tagged = true
			break
		}
	}
	if !tagged {
		t.Error("sessions-by-tag fixture has no tagged project; the By-Tag view would degrade to the signpost")
	}
}

func TestFixtureNamesIncludesByTag(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-by-tag" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-by-tag", capture.FixtureNames())
	}
}

func TestSessionsPagedFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-paged")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-paged): %v", err)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	const wantCount = 100
	if len(sessions) != wantCount {
		t.Fatalf("sessions-paged has %d sessions, want %d (multi-page determinism)", len(sessions), wantCount)
	}
	seen := map[string]bool{}
	for i, s := range sessions {
		if s.Name == "" {
			t.Errorf("session[%d] has an empty name", i)
		}
		if seen[s.Name] {
			t.Errorf("session[%d] name %q is a duplicate (must be deterministic & unique)", i, s.Name)
		}
		seen[s.Name] = true
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

func TestFixtureNamesIncludesPaged(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-paged" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-paged", capture.FixtureNames())
	}
}

func TestSessionsInlineFlashFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-inline-flash")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-inline-flash): %v", err)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	wantNames := []string{
		"fab-flowx-explore",
		"agentic-workflows-codify",
		"flowx-7UKPZH",
		"aviva-proxy-qNyfEO",
	}
	if len(sessions) != len(wantNames) {
		t.Fatalf("sessions-inline-flash has %d sessions, want %d (reference set)", len(sessions), len(wantNames))
	}
	for i, want := range wantNames {
		if sessions[i].Name != want {
			t.Errorf("session[%d] = %q, want %q (reference order)", i, sessions[i].Name, want)
		}
	}

	const msg = "folio-Jiz4el closed externally — list updated"
	if got := fx.Deps(darkBuiltinTheme(t)).Capture.Flash; got != msg {
		t.Errorf("Deps().Capture.Flash = %q, want %q (seeded warning flash)", got, msg)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

func TestFixtureNamesIncludesInlineFlash(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-inline-flash" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-inline-flash", capture.FixtureNames())
	}
}

func TestSessionsNoTagsSignpostFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-no-tags-signpost")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-no-tags-signpost): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Tag mode)", got, want)
	}

	projects, err := fx.Deps(darkBuiltinTheme(t)).ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("sessions-no-tags-signpost fixture has no projects; the signpost gate needs a session→dir→project mapping to be meaningful")
	}
	for _, p := range projects {
		if len(p.Tags) > 0 {
			t.Errorf("project %q carries tags %v; the signpost fixture must have ZERO tags anywhere so anyTagsExist is false", p.Name, p.Tags)
		}
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	type want struct {
		name     string
		windows  int
		attached bool
	}
	wants := []want{
		{"fab-flowx-explore", 3, true},
		{"agentic-workflows-codify", 1, false},
		{"flowx-7UKPZH", 2, false},
		{"aviva-proxy-qNyfEO", 4, false},
	}
	if len(sessions) != len(wants) {
		t.Fatalf("sessions-no-tags-signpost has %d sessions, want %d (reference set)", len(sessions), len(wants))
	}
	for i, w := range wants {
		got := sessions[i]
		if got.Name != w.name {
			t.Errorf("session[%d].Name = %q, want %q", i, got.Name, w.name)
		}
		if got.Windows != w.windows {
			t.Errorf("session[%d].Windows = %d, want %d (%s)", i, got.Windows, w.windows, w.name)
		}
		if got.Attached != w.attached {
			t.Errorf("session[%d].Attached = %t, want %t (%s)", i, got.Attached, w.attached, w.name)
		}
	}
}

func TestFixtureNamesIncludesNoTagsSignpost(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-no-tags-signpost" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-no-tags-signpost", capture.FixtureNames())
	}
}

func TestProjectsFixture(t *testing.T) {
	fx, err := capture.FixtureByName("projects")
	if err != nil {
		t.Fatalf("FixtureByName(projects): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions (the tape types x to reach Projects)", m.ActivePage())
	}

	projects, err := fx.Deps(darkBuiltinTheme(t)).ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	const wantCount = 14
	if len(projects) != wantCount {
		t.Fatalf("projects fixture has %d projects, want %d (matches the reference count)", len(projects), wantCount)
	}
	seen := map[string]bool{}
	for i, p := range projects {
		if p.Name == "" {
			t.Errorf("project[%d] has an empty name", i)
		}
		if !strings.HasPrefix(p.Path, "/") {
			t.Errorf("project[%d] %q path %q is not an absolute real-looking path", i, p.Name, p.Path)
		}
		if seen[p.Name] {
			t.Errorf("project[%d] name %q is a duplicate (must be deterministic & unique)", i, p.Name)
		}
		seen[p.Name] = true
	}
	if projects[0].Name != "flow-v1-api" {
		t.Errorf("projects fixture first project = %q, want %q (the reference cursor row)", projects[0].Name, "flow-v1-api")
	}
}

func TestFixtureNamesIncludesProjects(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "projects" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include projects", capture.FixtureNames())
	}
}

func TestPreviewScreenFixture(t *testing.T) {
	fx, err := capture.FixtureByName("preview-screen")
	if err != nil {
		t.Fatalf("FixtureByName(preview-screen): %v", err)
	}

	m := tui.Build(fx.Deps(darkBuiltinTheme(t)))
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions (the tape presses Space to reach the preview)", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 || sessions[0].Name != "aviva-proxy-qNyfEO" {
		t.Fatalf("first session = %+v, want first to be aviva-proxy-qNyfEO", sessions)
	}

	groups, err := fx.Deps(darkBuiltinTheme(t)).Enumerator.ListWindowsAndPanesInSession("aviva-proxy-qNyfEO")
	if err != nil {
		t.Fatalf("Enumerator: %v", err)
	}
	if len(groups) != 1 || len(groups[0].PaneIndices) != 1 {
		t.Errorf("enumerator groups = %+v, want a single window with a single pane (Window 1/1 · Pane 1/1)", groups)
	}

	body, err := fx.Deps(darkBuiltinTheme(t)).Reader.Tail("any-pane-key")
	if err != nil {
		t.Fatalf("Reader.Tail: %v", err)
	}
	if len(body) == 0 {
		t.Error("preview-screen scrollback is empty; the overlay would render the (no saved content) placeholder")
	}
	for _, banned := range []string{"Claude", "Fable", "Brewed"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("preview-screen scrollback references %q; the captured content must be generic, tool-agnostic example output", banned)
		}
	}
}

func TestFixtureNamesIncludesPreviewScreen(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "preview-screen" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include preview-screen", capture.FixtureNames())
	}
}

func TestSessionsMultiSelectActiveFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-multi-select-active")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-multi-select-active): %v", err)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	type want struct {
		name     string
		windows  int
		attached bool
	}
	wants := []want{
		{"agentic-workflows-code-based", 3, true},
		{"agentic-workflows-codify", 2, false},
		{"fab-flowx-explore", 1, false},
		{"evvi webhooks and watchers", 4, false},
		{"aviva-proxy-qNyfEO", 1, false},
		{"designlab-web-r8suyU", 2, false},
		{"evvi-sync-engine", 1, false},
		{"fab-aws-migration", 5, false},
		{"flow-v1-api-XkkhTN", 1, false},
		{"flowx-7UKPZH", 2, false},
		{"fabric-lk26UG", 1, false},
		{"folio-Jiz4el", 1, false},
	}
	if len(sessions) != len(wants) {
		t.Fatalf("sessions-multi-select-active has %d sessions, want %d (the sessions-flat set)", len(sessions), len(wants))
	}
	for i, w := range wants {
		got := sessions[i]
		if got.Name != w.name || got.Windows != w.windows || got.Attached != w.attached {
			t.Errorf("session[%d] = {%q,%d,%t}, want {%q,%d,%t}", i, got.Name, got.Windows, got.Attached, w.name, w.windows, w.attached)
		}
	}

	deps := fx.Deps(darkBuiltinTheme(t))
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.Capture.MultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().Capture.MultiSelect = %v, want %v", deps.Capture.MultiSelect, wantMarked)
	}
	for i, w := range wantMarked {
		if deps.Capture.MultiSelect[i] != w {
			t.Errorf("Deps().Capture.MultiSelect[%d] = %q, want %q", i, deps.Capture.MultiSelect[i], w)
		}
	}
	if got, want := deps.Capture.Cursor, "fab-flowx-explore"; got != want {
		t.Errorf("Deps().Capture.Cursor = %q, want %q (cursor on a marked, banded row)", got, want)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
	if !m.MultiSelectActive() {
		t.Error("MultiSelectActive() = false, want true (the fixture must open in multi-select mode)")
	}
	if got := m.SelectedSessionCount(); got != len(wantMarked) {
		t.Errorf("SelectedSessionCount() = %d, want %d", got, len(wantMarked))
	}
	for _, n := range wantMarked {
		if !m.IsSessionSelected(n) {
			t.Errorf("IsSessionSelected(%q) = false, want true", n)
		}
	}
}

func TestFixtureNamesIncludesMultiSelectActive(t *testing.T) {
	found := false
	for _, n := range capture.FixtureNames() {
		if n == "sessions-multi-select-active" {
			found = true
		}
	}
	if !found {
		t.Errorf("FixtureNames() %v does not include sessions-multi-select-active", capture.FixtureNames())
	}
}

type flatFixtureWant struct {
	name     string
	windows  int
	attached bool
}

func flatFixtureWants() []flatFixtureWant {
	return []flatFixtureWant{
		{"agentic-workflows-code-based", 3, true},
		{"agentic-workflows-codify", 2, false},
		{"fab-flowx-explore", 1, false},
		{"evvi webhooks and watchers", 4, false},
		{"aviva-proxy-qNyfEO", 1, false},
		{"designlab-web-r8suyU", 2, false},
		{"evvi-sync-engine", 1, false},
		{"fab-aws-migration", 5, false},
		{"flow-v1-api-XkkhTN", 1, false},
		{"flowx-7UKPZH", 2, false},
		{"fabric-lk26UG", 1, false},
		{"folio-Jiz4el", 1, false},
	}
}

func assertFlatFixtureSet(t *testing.T, fx *capture.Fixture) {
	t.Helper()
	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	wants := flatFixtureWants()
	if len(sessions) != len(wants) {
		t.Fatalf("fixture has %d sessions, want %d (the sessions-flat set)", len(sessions), len(wants))
	}
	for i, w := range wants {
		got := sessions[i]
		if got.Name != w.name || got.Windows != w.windows || got.Attached != w.attached {
			t.Errorf("session[%d] = {%q,%d,%t}, want {%q,%d,%t}", i, got.Name, got.Windows, got.Attached, w.name, w.windows, w.attached)
		}
	}
}

func assertFixtureNameListed(t *testing.T, name string) {
	t.Helper()
	if !slices.Contains(capture.FixtureNames(), name) {
		t.Errorf("FixtureNames() %v does not include %s", capture.FixtureNames(), name)
	}
}

func TestSessionsUnsupportedTerminalFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-unsupported-terminal")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-unsupported-terminal): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps(darkBuiltinTheme(t))
	if deps.Capture.Detection == nil {
		t.Fatal("Deps().Capture.Detection = nil, want a seeded Apple Terminal identity")
	}
	if got, want := deps.Capture.Detection.Name, "Apple Terminal"; got != want {
		t.Errorf("Deps().Capture.Detection.Name = %q, want %q", got, want)
	}
	if got, want := deps.Capture.Detection.BundleID, "com.apple.Terminal"; got != want {
		t.Errorf("Deps().Capture.Detection.BundleID = %q, want %q", got, want)
	}
	if len(deps.Capture.MultiSelect) != 0 {
		t.Errorf("Deps().Capture.MultiSelect = %v, want empty (NORMAL mode)", deps.Capture.MultiSelect)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if !m.DetectUnsupported() {
		t.Error("DetectUnsupported() = false, want true (Apple Terminal resolves unsupported → the banner renders)")
	}
	if m.MultiSelectActive() {
		t.Error("MultiSelectActive() = true, want false (the unsupported banner is proactive over the normal list)")
	}
}

func TestFixtureNamesIncludesUnsupportedTerminal(t *testing.T) {
	assertFixtureNameListed(t, "sessions-unsupported-terminal")
}

func TestSessionsUnsupportedNullFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-unsupported-null")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-unsupported-null): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps(darkBuiltinTheme(t))
	if deps.Capture.Detection == nil {
		t.Fatal("Deps().Capture.Detection = nil, want a seeded empty (NULL) identity")
	}
	if got := deps.Capture.Detection.BundleID; got != "" {
		t.Errorf("Deps().Capture.Detection.BundleID = %q, want \"\" (NULL identity)", got)
	}
	if len(deps.Capture.MultiSelect) != 0 {
		t.Errorf("Deps().Capture.MultiSelect = %v, want empty (NORMAL mode)", deps.Capture.MultiSelect)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if !m.DetectResolved() {
		t.Error("DetectResolved() = false, want true (the NULL identity is seeded resolved)")
	}
	if !m.DetectUnsupported() {
		t.Error("DetectUnsupported() = false, want true (a NULL identity resolves unsupported)")
	}
	if !m.DetectedIdentity().IsNull() {
		t.Error("DetectedIdentity().IsNull() = false, want true (empty BundleID → NULL)")
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (Flat — standard header, no banner)", got, want)
	}
	if m.MultiSelectActive() {
		t.Error("MultiSelectActive() = true, want false (the NULL case is the normal list)")
	}
}

func TestFixtureNamesIncludesUnsupportedNull(t *testing.T) {
	assertFixtureNameListed(t, "sessions-unsupported-null")
}

func TestSessionsMultiSelectPreflightAbortFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-multi-select-preflight-abort")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-multi-select-preflight-abort): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps(darkBuiltinTheme(t))
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.Capture.MultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().Capture.MultiSelect = %v, want %v", deps.Capture.MultiSelect, wantMarked)
	}
	for i, w := range wantMarked {
		if deps.Capture.MultiSelect[i] != w {
			t.Errorf("Deps().Capture.MultiSelect[%d] = %q, want %q", i, deps.Capture.MultiSelect[i], w)
		}
	}
	if got, want := deps.Capture.Cursor, "fab-flowx-explore"; got != want {
		t.Errorf("Deps().Capture.Cursor = %q, want %q (cursor on the gone row)", got, want)
	}
	wantGone := []string{"fab-flowx-explore"}
	if len(deps.Capture.GoneFlagged) != len(wantGone) || deps.Capture.GoneFlagged[0] != wantGone[0] {
		t.Errorf("Deps().Capture.GoneFlagged = %v, want %v", deps.Capture.GoneFlagged, wantGone)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if !m.MultiSelectActive() {
		t.Error("MultiSelectActive() = false, want true (survivors stay marked)")
	}
}

func TestFixtureNamesIncludesMultiSelectPreflightAbort(t *testing.T) {
	assertFixtureNameListed(t, "sessions-multi-select-preflight-abort")
}

func TestSessionsBurstOpeningFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-burst-opening")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-burst-opening): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps(darkBuiltinTheme(t))
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.Capture.MultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().Capture.MultiSelect = %v, want %v", deps.Capture.MultiSelect, wantMarked)
	}
	if got, want := deps.Capture.BurstOpening, [2]int{2, 3}; got != want {
		t.Errorf("Deps().Capture.BurstOpening = %v, want %v (Opening 2/3…)", got, want)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if !m.BurstPending() {
		t.Error("BurstPending() = false, want true (the Opening band must render)")
	}
	if got, want := m.BurstDone(), 2; got != want {
		t.Errorf("BurstDone() = %d, want %d", got, want)
	}
	if got, want := m.BurstTotal(), 3; got != want {
		t.Errorf("BurstTotal() = %d, want %d", got, want)
	}
}

func TestFixtureNamesIncludesBurstOpening(t *testing.T) {
	assertFixtureNameListed(t, "sessions-burst-opening")
}

func TestFakeSeamsAreInert(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-flat")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-flat): %v", err)
	}
	d := fx.Deps(darkBuiltinTheme(t))

	if err := d.Killer.KillSession("anything"); err != nil {
		t.Errorf("Killer.KillSession returned %v, want nil (no-op)", err)
	}
	if err := d.Renamer.RenameSession("a", "b"); err != nil {
		t.Errorf("Renamer.RenameSession returned %v, want nil (no-op)", err)
	}
	if _, err := d.Creator.CreateFromDir("/x", nil); err != nil {
		t.Errorf("Creator.CreateFromDir returned %v, want nil (no-op)", err)
	}

	groups, err := d.Enumerator.ListWindowsAndPanesInSession("agentic-workflows-code-based")
	if err != nil {
		t.Errorf("Enumerator returned %v, want nil", err)
	}
	if len(groups) == 0 {
		t.Error("Enumerator returned no window groups, want canned data")
	}
	if _, err := d.Reader.Tail("any-pane-key"); err != nil {
		t.Errorf("Reader.Tail returned %v, want nil", err)
	}
}

// The refusal wording is never seeded: the fixtures type into the real rename
// modal, so the frame carries whatever the production band composes.
func TestSessionsRenameRefusedFixtures(t *testing.T) {
	cases := []struct {
		fixture string
		flash   string
	}{
		{
			fixture: "sessions-rename-refused-separator",
			flash:   `":" isn't allowed in a session name — tmux reads it as a separator`,
		},
		{
			fixture: "sessions-rename-refused-id-prefix",
			flash:   `"$" isn't allowed at the start of a session name — tmux reads it as a session ID`,
		},
		{
			fixture: "sessions-rename-refused-flag-prefix",
			flash:   `"-" isn't allowed at the start of a session name — tmux reads it as a command flag`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			fx, err := capture.FixtureByName(tc.fixture)
			if err != nil {
				t.Fatalf("FixtureByName(%s): %v", tc.fixture, err)
			}
			if got := fx.Deps(darkBuiltinTheme(t)).Capture.Flash; got != "" {
				t.Errorf("Deps().Capture.Flash = %q, want empty — the refusal must be typed, not seeded", got)
			}

			m := fx.ModelAt(darkBuiltinTheme(t), harnessWidth, harnessHeight)
			frame := ansi.Strip(m.View().Content)
			if !strings.Contains(frame, tc.flash) {
				t.Errorf("captured frame is missing the refusal %q:\n%s", tc.flash, frame)
			}
		})
	}
}

func TestFixtureNamesIncludesRenameRefusals(t *testing.T) {
	for _, want := range []string{"sessions-rename-refused-separator", "sessions-rename-refused-id-prefix", "sessions-rename-refused-flag-prefix"} {
		if !slices.Contains(capture.FixtureNames(), want) {
			t.Errorf("FixtureNames() %v does not include %s", capture.FixtureNames(), want)
		}
	}
}
