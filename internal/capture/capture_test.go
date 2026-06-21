package capture_test

import (
	"testing"

	"github.com/leeovery/portal/internal/capture"
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
