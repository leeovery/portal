package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// mustMkdir creates dir (and parents) so CanonicalDirKey's EvalSymlinks
// resolves against a real on-disk path, keeping test keys deterministic.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
}

// asSessionItem unwraps a list.Item into a SessionItem, failing the test if the
// item is not a SessionItem (no header should ever appear in the slice).
func asSessionItem(t *testing.T, item list.Item) SessionItem {
	t.Helper()
	si, ok := item.(SessionItem)
	if !ok {
		t.Fatalf("item %v is not a SessionItem", item)
	}
	return si
}

func TestBuildByProject(t *testing.T) {
	t.Run("builds one item per session under its project name heading", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.Session.Name != "portal-abc" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "portal-abc")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
		if si.Tag != "" {
			t.Errorf("Tag = %q, want empty", si.Tag)
		}
		if si.CatchAll {
			t.Errorf("CatchAll = true, want false")
		}
	})

	t.Run("orders items by group key then session name", func(t *testing.T) {
		base := t.TempDir()
		dirA := filepath.Join(base, "a")
		dirB := filepath.Join(base, "b")
		mustMkdir(t, dirA)
		mustMkdir(t, dirB)
		keyA := project.CanonicalDirKey(dirA)
		keyB := project.CanonicalDirKey(dirB)

		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		// Deliberately unsorted input: B before A, and within A z before a.
		sessions := []tmux.Session{
			{Name: "bravo-1", Dir: dirB},
			{Name: "alpha-z", Dir: dirA},
			{Name: "alpha-a", Dir: dirA},
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		wantOrder := []struct {
			key  string
			name string
		}{
			{keyA, "alpha-a"},
			{keyA, "alpha-z"},
			{keyB, "bravo-1"},
		}
		for i, want := range wantOrder {
			si := asSessionItem(t, items[i])
			if si.GroupKey != want.key || si.Session.Name != want.name {
				t.Errorf("item[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, want.key, want.name)
			}
		}
	})

	t.Run("forms two separate groups for two distinct dirs sharing a project name", func(t *testing.T) {
		base := t.TempDir()
		dir1 := filepath.Join(base, "code", "portal")
		dir2 := filepath.Join(base, "archive", "portal")
		mustMkdir(t, dir1)
		mustMkdir(t, dir2)
		key1 := project.CanonicalDirKey(dir1)
		key2 := project.CanonicalDirKey(dir2)

		projects := []project.Project{
			{Path: dir1, Name: "Portal"},
			{Path: dir2, Name: "Portal"},
		}
		sessions := []tmux.Session{
			{Name: "s1", Dir: dir1},
			{Name: "s2", Dir: dir2},
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		keys := map[string]bool{}
		for _, it := range items {
			si := asSessionItem(t, it)
			if si.GroupHeading != "Portal" {
				t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
			}
			keys[si.GroupKey] = true
		}
		if !keys[key1] || !keys[key2] {
			t.Errorf("expected two distinct group keys %q and %q, got %v", key1, key2, keys)
		}
		if len(keys) != 2 {
			t.Errorf("len(distinct keys) = %d, want 2", len(keys))
		}
	})

	t.Run("routes a session with empty Dir to the Unknown bucket", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByProject(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Unknown" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Unknown")
		}
		if si.Session.Name != "no-dir" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "no-dir")
		}
	})

	t.Run("routes a stamped dir with no matching project record to the Unknown bucket", func(t *testing.T) {
		dir := t.TempDir()
		// No project record for dir → deleted project / live session.
		sessions := []tmux.Session{{Name: "orphan", Dir: dir}}

		items := buildByProject(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Unknown" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Unknown")
		}
	})

	t.Run("appends Unknown items after known-project items", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Known"}}
		sessions := []tmux.Session{
			{Name: "unknown-1"},
			{Name: "known-1", Dir: dir},
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		first := asSessionItem(t, items[0])
		if first.CatchAll {
			t.Errorf("first item is CatchAll, want known-project item first")
		}
		last := asSessionItem(t, items[1])
		if !last.CatchAll {
			t.Errorf("last item is not CatchAll, want Unknown item last")
		}
	})

	t.Run("returns an empty slice for zero live sessions", func(t *testing.T) {
		items := buildByProject(nil, project.NewIndex(nil))

		if len(items) != 0 {
			t.Fatalf("len(items) = %d, want 0", len(items))
		}
	})

	t.Run("suppresses the Unknown heading when no session is unresolvable", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "s1", Dir: dir},
			{Name: "s2", Dir: dir},
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		for _, it := range items {
			si := asSessionItem(t, it)
			if si.CatchAll {
				t.Errorf("unexpected catch-all item %+v; Unknown heading should be suppressed", si)
			}
			if si.GroupHeading == unknownHeading {
				t.Errorf("unexpected Unknown heading; should be suppressed when no session is unresolvable")
			}
		}
	})

	t.Run("orders Unknown catch-all items by session name", func(t *testing.T) {
		// Unsorted, all unresolvable (empty Dir).
		sessions := []tmux.Session{
			{Name: "charlie"},
			{Name: "alpha"},
			{Name: "bravo"},
		}

		items := buildByProject(sessions, project.NewIndex(nil))

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		wantNames := []string{"alpha", "bravo", "charlie"}
		for i, want := range wantNames {
			si := asSessionItem(t, items[i])
			if si.Session.Name != want {
				t.Errorf("item[%d].Session.Name = %q, want %q", i, si.Session.Name, want)
			}
		}
	})

	t.Run("stamps each Unknown catch-all item GroupKey with the heading constant", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByProject(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupKey != unknownHeading {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, unknownHeading)
		}
	})

	t.Run("pins the Unknown catch-all group last after alphabetical project headings", func(t *testing.T) {
		// A project named "Zulu" sorts after Unknown alphabetically; the catch-all
		// must still be pinned last (append-position, not alphabetical).
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Zulu"}}
		sessions := []tmux.Session{
			{Name: "orphan"}, // empty Dir → Unknown
			{Name: "zulu-1", Dir: dir},
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		first := asSessionItem(t, items[0])
		if first.GroupHeading != "Zulu" {
			t.Errorf("first item heading = %q, want %q", first.GroupHeading, "Zulu")
		}
		last := asSessionItem(t, items[1])
		if !last.CatchAll || last.GroupHeading != unknownHeading {
			t.Errorf("last item = %+v, want Unknown catch-all pinned last", last)
		}
	})

	t.Run("never drops a session: every input session appears exactly once", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "resolved", Dir: dir},
			{Name: "unresolvable-dir"},                  // empty Dir → Unknown
			{Name: "deleted-project", Dir: t.TempDir()}, // stamped, no record → Unknown
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		counts := map[string]int{}
		for _, it := range items {
			counts[asSessionItem(t, it).Session.Name]++
		}
		for _, s := range sessions {
			if counts[s.Name] != 1 {
				t.Errorf("session %q appears %d times, want exactly 1", s.Name, counts[s.Name])
			}
		}
	})

	t.Run("routes both an unresolvable-dir and a deleted-project session to Unknown", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "unresolvable-dir"},                  // empty Dir
			{Name: "deleted-project", Dir: t.TempDir()}, // stamped dir, no matching record
		}

		items := buildByProject(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		for _, it := range items {
			si := asSessionItem(t, it)
			if !si.CatchAll || si.GroupHeading != unknownHeading {
				t.Errorf("session %q not routed to Unknown: %+v", si.Session.Name, si)
			}
		}
	})
}

func TestBuildByTag(t *testing.T) {
	t.Run("emits one item per tag for a multi-tag session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		got := map[string]SessionItem{}
		for _, it := range items {
			si := asSessionItem(t, it)
			if si.CatchAll {
				t.Errorf("item for tag %q is CatchAll, want false", si.Tag)
			}
			if si.GroupKey != si.Tag {
				t.Errorf("GroupKey = %q, want = Tag %q", si.GroupKey, si.Tag)
			}
			if si.GroupHeading != si.Tag {
				t.Errorf("GroupHeading = %q, want = Tag %q", si.GroupHeading, si.Tag)
			}
			got[si.Tag] = si
		}
		for _, tag := range []string{"personal", "work"} {
			si, ok := got[tag]
			if !ok {
				t.Fatalf("missing item for tag %q, got %v", tag, got)
			}
			if si.Session.Name != "portal-abc" {
				t.Errorf("tag %q: Session.Name = %q, want %q", tag, si.Session.Name, "portal-abc")
			}
		}
	})

	t.Run("collapses work, Work and WORK into a single tag heading", func(t *testing.T) {
		base := t.TempDir()
		dir1 := filepath.Join(base, "one")
		dir2 := filepath.Join(base, "two")
		dir3 := filepath.Join(base, "three")
		mustMkdir(t, dir1)
		mustMkdir(t, dir2)
		mustMkdir(t, dir3)
		// Non-canonical stored values prove the defensive NormaliseTag collapse.
		projects := []project.Project{
			{Path: dir1, Name: "P1", Tags: []string{"work"}},
			{Path: dir2, Name: "P2", Tags: []string{"Work"}},
			{Path: dir3, Name: "P3", Tags: []string{"WORK"}},
		}
		sessions := []tmux.Session{
			{Name: "s1", Dir: dir1},
			{Name: "s2", Dir: dir2},
			{Name: "s3", Dir: dir3},
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		for _, it := range items {
			si := asSessionItem(t, it)
			if si.GroupKey != "work" {
				t.Errorf("GroupKey = %q, want %q", si.GroupKey, "work")
			}
			if si.GroupHeading != "work" {
				t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "work")
			}
		}
	})

	t.Run("emits exactly one Untagged item for a zero-tag session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "no-tags", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Untagged" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Untagged")
		}
		if si.Tag != "" {
			t.Errorf("Tag = %q, want empty", si.Tag)
		}
		if si.Session.Name != "no-tags" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "no-tags")
		}
	})

	t.Run("emits one Untagged item for a session whose dir has no project record", func(t *testing.T) {
		dir := t.TempDir()
		// No project record for dir → no tags → Untagged.
		sessions := []tmux.Session{{Name: "orphan", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Untagged" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Untagged")
		}
	})

	t.Run("emits one Untagged item for a session with empty Dir", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByTag(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Untagged" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Untagged")
		}
	})

	t.Run("skips a junk non-canonical stored tag without emitting an item", func(t *testing.T) {
		dir := t.TempDir()
		// "  " normalises to ok==false → skipped; "work" remains.
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"   ", "work"}}}
		sessions := []tmux.Session{{Name: "s1", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupKey != "work" || si.Tag != "work" {
			t.Errorf("item = (GroupKey %q, Tag %q), want both %q", si.GroupKey, si.Tag, "work")
		}
		if si.CatchAll {
			t.Errorf("CatchAll = true, want false")
		}
	})

	t.Run("routes a project whose only tag is junk to Untagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"   "}}}
		sessions := []tmux.Session{{Name: "s1", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Untagged" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Untagged")
		}
	})

	t.Run("orders tagged items by canonical tag then session name", func(t *testing.T) {
		base := t.TempDir()
		dir1 := filepath.Join(base, "one")
		dir2 := filepath.Join(base, "two")
		mustMkdir(t, dir1)
		mustMkdir(t, dir2)
		projects := []project.Project{
			{Path: dir1, Name: "P1", Tags: []string{"work", "alpha"}},
			{Path: dir2, Name: "P2", Tags: []string{"alpha"}},
		}
		// Unsorted input: ensure ordering is by (tag, name), not input order.
		sessions := []tmux.Session{
			{Name: "z-sess", Dir: dir1},
			{Name: "a-sess", Dir: dir2},
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		wantOrder := []struct {
			tag  string
			name string
		}{
			{"alpha", "a-sess"},
			{"alpha", "z-sess"},
			{"work", "z-sess"},
		}
		for i, want := range wantOrder {
			si := asSessionItem(t, items[i])
			if si.GroupKey != want.tag || si.Session.Name != want.name {
				t.Errorf("item[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, want.tag, want.name)
			}
		}
	})

	t.Run("appends Untagged items after tagged items", func(t *testing.T) {
		base := t.TempDir()
		tagged := filepath.Join(base, "tagged")
		mustMkdir(t, tagged)
		projects := []project.Project{{Path: tagged, Name: "Tagged", Tags: []string{"work"}}}
		sessions := []tmux.Session{
			{Name: "untagged-1"},
			{Name: "tagged-1", Dir: tagged},
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		first := asSessionItem(t, items[0])
		if first.CatchAll {
			t.Errorf("first item is CatchAll, want tagged item first")
		}
		last := asSessionItem(t, items[1])
		if !last.CatchAll {
			t.Errorf("last item is not CatchAll, want Untagged item last")
		}
	})

	t.Run("shares the underlying session across a multi-tag session's instances", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir, Windows: 3, Attached: true}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		a := asSessionItem(t, items[0])
		b := asSessionItem(t, items[1])
		if a.Session != b.Session {
			t.Errorf("instances reference different sessions: %+v vs %+v", a.Session, b.Session)
		}
		if a.Session != sessions[0] {
			t.Errorf("instance session = %+v, want %+v", a.Session, sessions[0])
		}
	})

	t.Run("sum of items exceeds live session count when a session is multi-tagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal", "urgent"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) <= len(sessions) {
			t.Errorf("len(items) = %d, want > live session count %d", len(items), len(sessions))
		}
	})

	t.Run("returns an empty slice for zero live sessions", func(t *testing.T) {
		items := buildByTag(nil, project.NewIndex(nil))

		if len(items) != 0 {
			t.Fatalf("len(items) = %d, want 0", len(items))
		}
	})

	t.Run("suppresses the Untagged heading when every session is tagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{
			{Name: "s1", Dir: dir},
			{Name: "s2", Dir: dir},
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		for _, it := range items {
			si := asSessionItem(t, it)
			if si.CatchAll {
				t.Errorf("unexpected catch-all item %+v; Untagged heading should be suppressed", si)
			}
			if si.GroupHeading == untaggedHeading {
				t.Errorf("unexpected Untagged heading; should be suppressed when every session is tagged")
			}
		}
	})

	t.Run("orders Untagged catch-all items by session name", func(t *testing.T) {
		// Unsorted, all untagged (empty Dir).
		sessions := []tmux.Session{
			{Name: "charlie"},
			{Name: "alpha"},
			{Name: "bravo"},
		}

		items := buildByTag(sessions, project.NewIndex(nil))

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		wantNames := []string{"alpha", "bravo", "charlie"}
		for i, want := range wantNames {
			si := asSessionItem(t, items[i])
			if si.Session.Name != want {
				t.Errorf("item[%d].Session.Name = %q, want %q", i, si.Session.Name, want)
			}
		}
	})

	t.Run("stamps each Untagged catch-all item GroupKey with the heading constant", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByTag(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupKey != untaggedHeading {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, untaggedHeading)
		}
	})

	t.Run("pins the Untagged catch-all group last after alphabetical tag headings", func(t *testing.T) {
		// Tag "zeta" sorts after Untagged alphabetically; the catch-all must still
		// be pinned last (append-position, not alphabetical).
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"zeta"}}}
		sessions := []tmux.Session{
			{Name: "orphan"}, // empty Dir → Untagged
			{Name: "zeta-1", Dir: dir},
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		first := asSessionItem(t, items[0])
		if first.GroupHeading != "zeta" {
			t.Errorf("first item heading = %q, want %q", first.GroupHeading, "zeta")
		}
		last := asSessionItem(t, items[1])
		if !last.CatchAll || last.GroupHeading != untaggedHeading {
			t.Errorf("last item = %+v, want Untagged catch-all pinned last", last)
		}
	})

	t.Run("routes a deleted-project session to Untagged", func(t *testing.T) {
		// Stamped dir, no matching project record → no tags → Untagged.
		sessions := []tmux.Session{{Name: "deleted-project", Dir: t.TempDir()}}

		items := buildByTag(sessions, project.NewIndex(nil))

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll || si.GroupHeading != untaggedHeading {
			t.Errorf("deleted-project session not routed to Untagged: %+v", si)
		}
	})

	t.Run("never drops a session: every input session appears at least once", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal"}}}
		sessions := []tmux.Session{
			{Name: "tagged", Dir: dir},
			{Name: "untagged-dir"},                      // empty Dir → Untagged
			{Name: "deleted-project", Dir: t.TempDir()}, // stamped, no record → Untagged
		}

		items := buildByTag(sessions, project.NewIndex(projects))

		seen := map[string]bool{}
		for _, it := range items {
			seen[asSessionItem(t, it).Session.Name] = true
		}
		for _, s := range sessions {
			if !seen[s.Name] {
				t.Errorf("session %q dropped from rendered set", s.Name)
			}
		}
	})
}
