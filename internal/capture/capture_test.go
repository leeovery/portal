package capture_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

// TestFixtureByName resolves the named fixtures and verifies the deterministic
// session set wired into the sessions-flat fixture matches the Paper-mock list
// (spec § 15 / tick task), in order, with the correct window counts and
// attached flags.
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
		// The exact ordered set named in the tick task (load-bearing for the
		// deterministic capture).
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

		// The fixture exposes its seam set as a tui.Deps so the harness builds
		// the production model — no bespoke render path.
		m := tui.Build(fx.Deps())
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
	})
}

// TestSessionsEmptyFixture verifies the §11.1 empty-sessions fixture: ZERO sessions
// (an empty Lister) opened in Flat mode, so the empty-sessions state renders. The
// render assertion (glyph / message / hint / replaced footer styling) lives in
// internal/tui; here the gate is that the fixture wires ZERO sessions through to the
// production Sessions model (the precondition the empty state needs).
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

	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

// TestFixtureNamesIncludesSessionsEmpty pins the empty-sessions fixture into the
// discoverable name list (the --fixture help + FixtureByName error share this source).
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

// TestLoadingScreenFixture verifies the §10 loading-screen fixture: it parks the
// production model on PageLoading (serverStarted) and is registered in the
// discoverable name list. The render assertions (block banner / bar / step-list /
// tokens / counter) live in internal/tui; here the gate is that the fixture wires
// the loading page through to the production model so the capture can screenshot
// it.
func TestLoadingScreenFixture(t *testing.T) {
	fx, err := capture.FixtureByName("loading-screen")
	if err != nil {
		t.Fatalf("FixtureByName(loading-screen): %v", err)
	}

	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading (the loading-screen fixture must park on the loading page)", m.ActivePage())
	}

	// The progress receiver is wired (the seeded mid-restore events stream through
	// the real Update path), so Build does NOT synthesize the terminal complete
	// event — the page stays on PageLoading for the capture.
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

// TestLoadingErrorFixture verifies the §10.5 fatal cold-boot error fixture
// (MOCKED — no §15.1 Paper oracle): it parks the production model on PageLoading
// (serverStarted), and driving its seeded receiver events (steps 1–2 done, then a
// terminal fatal at step 3) through the real Update path enters the in-TUI error
// state — the failed step's row renders the state.red ✗, the one-line message + a
// quit hint render beneath the step-list, and the page never transitions to the
// picker. It is registered in the discoverable name list.
func TestLoadingErrorFixture(t *testing.T) {
	fx, err := capture.FixtureByName("loading-error")
	if err != nil {
		t.Fatalf("FixtureByName(loading-error): %v", err)
	}

	// Pin the dark appearance (as the capturetool does for --appearance dark) so the
	// detect-or-timeout first-paint gate is already resolved and View() paints the
	// real frame rather than the neutral held blank.
	deps := fx.Deps()
	deps.Appearance = prefs.AppearanceDark
	m := tui.Build(deps)
	if m.ActivePage() != tui.PageLoading {
		t.Errorf("ActivePage() = %d, want PageLoading (the loading-error fixture must park on the loading page)", m.ActivePage())
	}

	// Drive the seeded receiver events through the real Update path: steps 1–2
	// (progress) then the terminal fatal. The fixture wires loadingFatalReceiver,
	// so each pull yields the next seeded msg in order.
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

// TestSessionsByProjectFixture verifies the grouped (by-project) fixture: it
// opens in ModeByProject and carries projects whose paths match most session dirs,
// plus exactly one session whose directory matches NO project so it lands in the
// pinned Unknown catch-all. This is the fixture that drives the §5.2 By-Project
// grouped capture (mode suffix + heading reskin + nested rows).
func TestSessionsByProjectFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-project")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-project): %v", err)
	}

	// It builds the production Sessions model opened in By-Project mode.
	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions — by project"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Project mode)", got, want)
	}

	// There must be MULTIPLE projects so several group headings render.
	projects, err := fx.Deps().ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	if len(projects) < 2 {
		t.Errorf("sessions-by-project fixture has %d projects, want >=2 (multiple group headings)", len(projects))
	}

	// At least one session's directory must match no project, so the Unknown
	// catch-all heading renders alongside the resolvable groups.
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

// TestFixtureNamesIncludesByProject pins the by-project fixture into the
// discoverable name list (the --fixture help + FixtureByName error share this
// source).
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

// TestSessionsByTagFixtureExercisesMultiTagAndUntagged pins the §5.3 Pattern B +
// catch-all coverage the by-tag capture must exercise: at least one project with
// MULTIPLE tags (so a session repeats under each tag heading) AND at least one
// session whose directory is untagged (so the Untagged catch-all heading renders).
func TestSessionsByTagFixtureExercisesMultiTagAndUntagged(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-tag")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-tag): %v", err)
	}
	projects, err := fx.Deps().ProjectStore.List()
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

// TestSessionsByTagFixture verifies the grouped (by-tag) fixture: it opens in
// ModeByTag and carries tagged projects whose paths match the session dirs, so the
// By-Tag grouping renders real `— by tag` headings (not the "No tags yet"
// signpost). This is the fixture that drives the mode-suffix capture.
func TestSessionsByTagFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-by-tag")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-by-tag): %v", err)
	}

	// It builds the production Sessions model opened in By-Tag mode.
	m := tui.Build(fx.Deps())
	if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Tag mode)", got, want)
	}

	// At least one project carries a tag, so the By-Tag view groups rather than
	// degrading to the "No tags yet" signpost.
	projects, err := fx.Deps().ProjectStore.List()
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

// TestFixtureNamesIncludesByTag pins the by-tag fixture into the discoverable
// name list (the --fixture help + FixtureByName error share this source).
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

// TestSessionsPagedFixture verifies the multi-page (sessions-paged) fixture: it
// carries enough deterministic sessions to span more than one page at the
// 1280×800 capture size, so the height-driven paginator renders the §3.5 dot row.
// Determinism (a fixed session count and names) is the capture gate.
func TestSessionsPagedFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-paged")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-paged): %v", err)
	}

	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	// 100 sessions spans several pages at the capture size (measured PerPage ≤ 31
	// there, so ~4 pages), so a multi-dot row renders. The count is fixed for determinism.
	const wantCount = 100
	if len(sessions) != wantCount {
		t.Fatalf("sessions-paged has %d sessions, want %d (multi-page determinism)", len(sessions), wantCount)
	}
	// Deterministic, unique names (no duplicates that could reorder a capture).
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

	// It builds the production Sessions model opened in Flat mode (the dots are a
	// Flat-list concern; By-Tag has its own fixture).
	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

// TestFixtureNamesIncludesPaged pins the multi-page fixture into the discoverable
// name list (the --fixture help + FixtureByName error share this source).
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

// TestSessionsInlineFlashFixture verifies the §11.2 inline-flash fixture: a small
// Flat-mode session set with the warning flash seeded so the band renders on the
// first frame (mirroring testdata/vhs/reference/sessions-inline-flash-mv.png). The
// session set matches the reference exactly and the flash message + ⚠ glyph appear
// in the rendered View.
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

	// The §11.2 warning flash is seeded into the Deps seam (the only way to render
	// the otherwise-transient band in the inert harness). The render assertion (band
	// styling, ⚠ glyph, bg.warning tint) lives in internal/tui; here we pin that the
	// fixture wires the message through to Build.
	const msg = "folio-Jiz4el closed externally — list updated"
	if got := fx.Deps().InitialFlash; got != msg {
		t.Errorf("Deps().InitialFlash = %q, want %q (seeded warning flash)", got, msg)
	}

	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}
}

// TestFixtureNamesIncludesInlineFlash pins the inline-flash fixture into the
// discoverable name list (the --fixture help + FixtureByName error share this source).
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

// TestSessionsNoTagsSignpostFixture verifies the §11.3 no-tags-signpost fixture:
// it opens in By-Tag mode with projects that carry NO tags, so anyTagsExist is
// false and the By-Tag view degrades to the flat list under the persistent violet
// info signpost (rather than grouping). The session set matches the reference
// frame exactly (testdata/vhs/reference/sessions-no-tags-signpost-mv.png).
func TestSessionsNoTagsSignpostFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-no-tags-signpost")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-no-tags-signpost): %v", err)
	}

	// It opens in By-Tag mode (the mode that drives the zero-tags signpost).
	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in By-Tag mode)", got, want)
	}

	// NO project carries a tag → the signpost shows over the flat list (degrade
	// with message, not silent flatten).
	projects, err := fx.Deps().ProjectStore.List()
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

	// The session set matches the reference frame exactly, in order, with the
	// reference window counts + attached flags.
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

// TestFixtureNamesIncludesNoTagsSignpost pins the no-tags-signpost fixture into
// the discoverable name list (the --fixture help + FixtureByName error share this
// source).
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

// TestProjectsFixture verifies the §6 Projects-page fixture: it opens on the
// Sessions page (the production default; the tape types `x` to reach Projects) and
// carries a rich project store (14 projects with real-looking absolute paths) so the
// Projects (MV) reskin capture has a representative project set. Determinism (a
// fixed project count, names, and paths) is the capture gate.
func TestProjectsFixture(t *testing.T) {
	fx, err := capture.FixtureByName("projects")
	if err != nil {
		t.Fatalf("FixtureByName(projects): %v", err)
	}

	// It builds the production model opened on the Sessions page (the tape reaches
	// Projects via the `x` key, mirroring a real no-arg launch).
	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions (the tape types x to reach Projects)", m.ActivePage())
	}

	projects, err := fx.Deps().ProjectStore.List()
	if err != nil {
		t.Fatalf("ProjectStore.List: %v", err)
	}
	const wantCount = 14
	if len(projects) != wantCount {
		t.Fatalf("projects fixture has %d projects, want %d (matches the reference count)", len(projects), wantCount)
	}
	// Deterministic, unique names and non-empty real-looking paths.
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
	// The reference's selected (cursor) row is flow-v1-api, so it must be first.
	if projects[0].Name != "flow-v1-api" {
		t.Errorf("projects fixture first project = %q, want %q (the reference cursor row)", projects[0].Name, "flow-v1-api")
	}
}

// TestFixtureNamesIncludesProjects pins the projects fixture into the discoverable
// name list (the --fixture help + FixtureByName error share this source).
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

// TestPreviewScreenFixture verifies the §9 preview-screen fixture: a Flat-mode
// session list whose FIRST (default-selected) session is aviva-proxy-qNyfEO, so
// pressing Space opens the §9 preview overlay onto a single-window single-pane
// session (Window 1/1 · Pane 1/1) seeded with generic canned scrollback. The
// render assertion (the §9.1 cyan peek-mode chrome) lives in internal/tui; here
// the gate is that the fixture wires the right session order, the single-pane
// enumerator shape, and the generic (tool-agnostic) scrollback through Deps.
func TestPreviewScreenFixture(t *testing.T) {
	fx, err := capture.FixtureByName("preview-screen")
	if err != nil {
		t.Fatalf("FixtureByName(preview-screen): %v", err)
	}

	// It builds the production Sessions model opened in Flat mode (the tape
	// presses Space to reach the preview overlay).
	m := tui.Build(fx.Deps())
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions (the tape presses Space to reach the preview)", m.ActivePage())
	}
	if got, want := m.SessionListTitle(), "Sessions"; got != want {
		t.Errorf("SessionListTitle() = %q, want %q (fixture opens in Flat mode)", got, want)
	}

	// The default-selected (first) session must be aviva-proxy-qNyfEO so Space
	// opens the preview onto the reference session.
	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 || sessions[0].Name != "aviva-proxy-qNyfEO" {
		t.Fatalf("first session = %+v, want first to be aviva-proxy-qNyfEO", sessions)
	}

	// The enumerator must be a single 1/1 window/pane so the §9.1 counters read
	// "Window 1/1 · Pane 1/1" (matching the reference frame).
	groups, err := fx.Deps().Enumerator.ListWindowsAndPanesInSession("aviva-proxy-qNyfEO")
	if err != nil {
		t.Fatalf("Enumerator: %v", err)
	}
	if len(groups) != 1 || len(groups[0].PaneIndices) != 1 {
		t.Errorf("enumerator groups = %+v, want a single window with a single pane (Window 1/1 · Pane 1/1)", groups)
	}

	// The scrollback is non-empty generic example output — and must NOT
	// reference any specific tool/model (Portal's preview is tool-agnostic).
	body, err := fx.Deps().Reader.Tail("any-pane-key")
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

// TestFixtureNamesIncludesPreviewScreen pins the preview-screen fixture into the
// discoverable name list (the --fixture help + FixtureByName error share this source).
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

// TestSessionsMultiSelectActiveFixture verifies the §5 multi-select-active fixture:
// it reuses the sessions-flat set (same 12 sessions, same order), opens in Flat
// mode, and seeds the multi-select seed seam — three marked sessions
// (agentic-workflows-codify / fab-flowx-explore / designlab-web-r8suyU) with the
// cursor anchored on fab-flowx-explore (a marked, banded row per the delivered
// frame). The render assertions (violet banner / ● markers / footer) live in the
// visual gate; here the gate is that the fixture wires the seed seam through Deps.
func TestSessionsMultiSelectActiveFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-multi-select-active")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-multi-select-active): %v", err)
	}

	// It reuses the sessions-flat set exactly (same names, order, window counts,
	// attached flags) — determinism is the capture gate.
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

	// The seed seam wires the three marked names + the cursor anchor through Deps.
	deps := fx.Deps()
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.InitialMultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().InitialMultiSelect = %v, want %v", deps.InitialMultiSelect, wantMarked)
	}
	for i, w := range wantMarked {
		if deps.InitialMultiSelect[i] != w {
			t.Errorf("Deps().InitialMultiSelect[%d] = %q, want %q", i, deps.InitialMultiSelect[i], w)
		}
	}
	if got, want := deps.InitialCursor, "fab-flowx-explore"; got != want {
		t.Errorf("Deps().InitialCursor = %q, want %q (cursor on a marked, banded row)", got, want)
	}

	// It builds the production Sessions model in Flat multi-select mode with the
	// three sessions marked.
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

// TestFixtureNamesIncludesMultiSelectActive pins the multi-select-active fixture
// into the discoverable name list (the --fixture help + FixtureByName error share
// this source).
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

// flatFixtureWants is the ordered sessions-flat set (names / window counts /
// attached flags) the §6 picker-burst fixtures reuse verbatim, asserted so a fixture
// that silently drifts from the shared set fails loudly.
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

// TestSessionsUnsupportedTerminalFixture verifies the §6.2 proactive
// unsupported-terminal fixture: it reuses the sessions-flat set (NORMAL mode, no
// multi-select) and seeds the detection cache with a non-NULL Apple Terminal
// identity, so the built model resolves unsupported and DetectUnsupported() is true
// (the proactive amber banner renders). The banner render assertion lives in
// internal/tui; here the gate is that the fixture wires the identity through Deps.
func TestSessionsUnsupportedTerminalFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-unsupported-terminal")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-unsupported-terminal): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps()
	if deps.InitialDetection == nil {
		t.Fatal("Deps().InitialDetection = nil, want a seeded Apple Terminal identity")
	}
	if got, want := deps.InitialDetection.Name, "Apple Terminal"; got != want {
		t.Errorf("Deps().InitialDetection.Name = %q, want %q", got, want)
	}
	if got, want := deps.InitialDetection.BundleID, "com.apple.Terminal"; got != want {
		t.Errorf("Deps().InitialDetection.BundleID = %q, want %q", got, want)
	}
	// It must NOT be in multi-select mode — the banner is proactive over the normal list.
	if len(deps.InitialMultiSelect) != 0 {
		t.Errorf("Deps().InitialMultiSelect = %v, want empty (NORMAL mode)", deps.InitialMultiSelect)
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

// TestFixtureNamesIncludesUnsupportedTerminal pins the unsupported-terminal fixture
// into the discoverable name list.
func TestFixtureNamesIncludesUnsupportedTerminal(t *testing.T) {
	assertFixtureNameListed(t, "sessions-unsupported-terminal")
}

// TestSessionsMultiSelectPreflightAbortFixture verifies the §6.7 pre-flight abort
// fixture: it reuses the sessions-flat set, opens in multi-select mode with the three
// marked sessions, anchors the cursor on fab-flowx-explore (the gone row), and seeds
// the gone-flag on fab-flowx-explore so the red abort banner + gone-row badge render
// while the survivors keep their ●. The render assertions live in the visual gate;
// here the gate is that the fixture wires the seed seams through Deps.
func TestSessionsMultiSelectPreflightAbortFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-multi-select-preflight-abort")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-multi-select-preflight-abort): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps()
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.InitialMultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().InitialMultiSelect = %v, want %v", deps.InitialMultiSelect, wantMarked)
	}
	for i, w := range wantMarked {
		if deps.InitialMultiSelect[i] != w {
			t.Errorf("Deps().InitialMultiSelect[%d] = %q, want %q", i, deps.InitialMultiSelect[i], w)
		}
	}
	if got, want := deps.InitialCursor, "fab-flowx-explore"; got != want {
		t.Errorf("Deps().InitialCursor = %q, want %q (cursor on the gone row)", got, want)
	}
	wantGone := []string{"fab-flowx-explore"}
	if len(deps.InitialGoneFlagged) != len(wantGone) || deps.InitialGoneFlagged[0] != wantGone[0] {
		t.Errorf("Deps().InitialGoneFlagged = %v, want %v", deps.InitialGoneFlagged, wantGone)
	}

	m := tui.Build(deps)
	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
	}
	if !m.MultiSelectActive() {
		t.Error("MultiSelectActive() = false, want true (survivors stay marked)")
	}
}

// TestFixtureNamesIncludesMultiSelectPreflightAbort pins the pre-flight abort fixture
// into the discoverable name list.
func TestFixtureNamesIncludesMultiSelectPreflightAbort(t *testing.T) {
	assertFixtureNameListed(t, "sessions-multi-select-preflight-abort")
}

// TestSessionsBurstOpeningFixture verifies the §6.5 in-burst Opening fixture: it
// reuses the sessions-flat set, opens in multi-select mode with the three marked
// sessions, and seeds the in-burst Opening band as (2, 3) so the built model renders
// the `Opening 2/3…` band (BurstPending with the done/total counters). The band
// render assertion lives in internal/tui; here the gate is that the fixture wires the
// seed seam through Deps.
func TestSessionsBurstOpeningFixture(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-burst-opening")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-burst-opening): %v", err)
	}

	assertFlatFixtureSet(t, fx)

	deps := fx.Deps()
	wantMarked := []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}
	if len(deps.InitialMultiSelect) != len(wantMarked) {
		t.Fatalf("Deps().InitialMultiSelect = %v, want %v", deps.InitialMultiSelect, wantMarked)
	}
	if got, want := deps.InitialBurstOpening, [2]int{2, 3}; got != want {
		t.Errorf("Deps().InitialBurstOpening = %v, want %v (Opening 2/3…)", got, want)
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

// TestFixtureNamesIncludesBurstOpening pins the in-burst Opening fixture into the
// discoverable name list.
func TestFixtureNamesIncludesBurstOpening(t *testing.T) {
	assertFixtureNameListed(t, "sessions-burst-opening")
}

// TestFakeSeamsAreInert verifies the mutating fakes are no-ops (the harness must
// never mutate any tmux/server/config state) and the read seams return canned
// data without touching a real tmux server.
func TestFakeSeamsAreInert(t *testing.T) {
	fx, err := capture.FixtureByName("sessions-flat")
	if err != nil {
		t.Fatalf("FixtureByName(sessions-flat): %v", err)
	}
	d := fx.Deps()

	if err := d.Killer.KillSession("anything"); err != nil {
		t.Errorf("Killer.KillSession returned %v, want nil (no-op)", err)
	}
	if err := d.Renamer.RenameSession("a", "b"); err != nil {
		t.Errorf("Renamer.RenameSession returned %v, want nil (no-op)", err)
	}
	if _, err := d.Creator.CreateFromDir("/x", nil); err != nil {
		t.Errorf("Creator.CreateFromDir returned %v, want nil (no-op)", err)
	}

	// The enumerator and reader return canned data deterministically.
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
