package tui

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/list"
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
// item is not a SessionItem.
func asSessionItem(t *testing.T, item list.Item) SessionItem {
	t.Helper()
	si, ok := item.(SessionItem)
	if !ok {
		t.Fatalf("item %v is not a SessionItem", item)
	}
	return si
}

// sessionRows returns only the SessionItem rows from a grouped item slice,
// dropping the injected HeaderItem separators. Grouped lists now interleave a
// HeaderItem before each group, so tests that assert on session membership /
// ordering filter the headers out with this helper.
func sessionRows(items []list.Item) []SessionItem {
	var out []SessionItem
	for _, it := range items {
		if si, ok := it.(SessionItem); ok {
			out = append(out, si)
		}
	}
	return out
}

// headerRows returns only the HeaderItem separators from a grouped item slice,
// in order — for tests that assert on group headings and their counts.
func headerRows(items []list.Item) []HeaderItem {
	var out []HeaderItem
	for _, it := range items {
		if h, ok := it.(HeaderItem); ok {
			out = append(out, h)
		}
	}
	return out
}

func TestBuildByProject(t *testing.T) {
	t.Run("builds one item per session under its project name heading", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByProject(sessions, project.NewIndex(projects))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.Session.Name != "portal-abc" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "portal-abc")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
		if si.CatchAll {
			t.Errorf("CatchAll = true, want false")
		}

		// The group's header is a real list item carrying the project name and a
		// row count of 1, interleaved before its session row.
		headers := headerRows(items)
		if len(headers) != 1 {
			t.Fatalf("len(headers) = %d, want 1", len(headers))
		}
		if headers[0].Heading != "Portal" || headers[0].Count != 1 {
			t.Errorf("header = (%q, %d), want (%q, 1)", headers[0].Heading, headers[0].Count, "Portal")
		}
	})

	t.Run("known-project GroupKey reuses the key returned by idx.Match", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		idx := project.NewIndex(projects)
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		// The GroupKey buildByProject stamps must be exactly the canonical key
		// idx.Match computes and returns for the same Dir — proving the key is
		// reused, not recomputed via a second CanonicalDirKey call.
		_, matchKey, ok := idx.Match(dir)
		if !ok {
			t.Fatalf("idx.Match(%q) ok = false, want true", dir)
		}

		items := buildByProject(sessions, idx)
		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.GroupKey != matchKey {
			t.Errorf("GroupKey = %q, want %q (key returned by idx.Match)", si.GroupKey, matchKey)
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

		rows := sessionRows(items)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
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
			si := rows[i]
			if si.GroupKey != want.key || si.Session.Name != want.name {
				t.Errorf("row[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, want.key, want.name)
			}
		}

		// One header per group, in (sorted) group order: Alpha (2 rows) then
		// Bravo (1 row).
		headers := headerRows(items)
		wantHeaders := []struct {
			heading string
			count   int
		}{
			{"Alpha", 2},
			{"Bravo", 1},
		}
		if len(headers) != len(wantHeaders) {
			t.Fatalf("len(headers) = %d, want %d", len(headers), len(wantHeaders))
		}
		for i, want := range wantHeaders {
			if headers[i].Heading != want.heading || headers[i].Count != want.count {
				t.Errorf("header[%d] = (%q, %d), want (%q, %d)", i, headers[i].Heading, headers[i].Count, want.heading, want.count)
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		keys := map[string]bool{}
		for _, si := range rows {
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

		// Two distinct dirs sharing the name "Portal" form two separate groups,
		// hence two headers (both labelled "Portal").
		headers := headerRows(items)
		if len(headers) != 2 {
			t.Errorf("len(headers) = %d, want 2 (one per distinct-dir group)", len(headers))
		}
		for _, h := range headers {
			if h.Heading != "Portal" {
				t.Errorf("header heading = %q, want %q", h.Heading, "Portal")
			}
		}
	})

	t.Run("routes a session with empty Dir to the Unknown bucket", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByProject(sessions, project.NewIndex(nil))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		first := rows[0]
		if first.CatchAll {
			t.Errorf("first row is CatchAll, want known-project item first")
		}
		last := rows[1]
		if !last.CatchAll {
			t.Errorf("last row is not CatchAll, want Unknown item last")
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

		for _, si := range sessionRows(items) {
			if si.CatchAll {
				t.Errorf("unexpected catch-all item %+v; Unknown heading should be suppressed", si)
			}
			if si.GroupHeading == unknownHeading {
				t.Errorf("unexpected Unknown heading; should be suppressed when no session is unresolvable")
			}
		}
		// Empty-suppression at the header layer: no Unknown HeaderItem is emitted
		// when no session is unresolvable.
		for _, h := range headerRows(items) {
			if h.Heading == unknownHeading {
				t.Errorf("unexpected Unknown header; should be suppressed when no session is unresolvable")
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

		rows := sessionRows(items)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		wantNames := []string{"alpha", "bravo", "charlie"}
		for i, want := range wantNames {
			si := rows[i]
			if si.Session.Name != want {
				t.Errorf("row[%d].Session.Name = %q, want %q", i, si.Session.Name, want)
			}
		}
	})

	t.Run("stamps each Unknown catch-all item GroupKey with the heading constant", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByProject(sessions, project.NewIndex(nil))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		first := rows[0]
		if first.GroupHeading != "Zulu" {
			t.Errorf("first row heading = %q, want %q", first.GroupHeading, "Zulu")
		}
		last := rows[1]
		if !last.CatchAll || last.GroupHeading != unknownHeading {
			t.Errorf("last row = %+v, want Unknown catch-all pinned last", last)
		}

		// The header order mirrors the row order: Zulu first, Unknown pinned last.
		headers := headerRows(items)
		if len(headers) != 2 {
			t.Fatalf("len(headers) = %d, want 2", len(headers))
		}
		if headers[0].Heading != "Zulu" {
			t.Errorf("first header heading = %q, want %q", headers[0].Heading, "Zulu")
		}
		if headers[1].Heading != unknownHeading {
			t.Errorf("last header heading = %q, want %q (Unknown pinned last)", headers[1].Heading, unknownHeading)
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
		for _, si := range sessionRows(items) {
			counts[si.Session.Name]++
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		for _, si := range rows {
			if !si.CatchAll || si.GroupHeading != unknownHeading {
				t.Errorf("session %q not routed to Unknown: %+v", si.Session.Name, si)
			}
		}
		// Both unresolvable sessions share a single Unknown header with Count 2.
		headers := headerRows(items)
		if len(headers) != 1 {
			t.Fatalf("len(headers) = %d, want 1", len(headers))
		}
		if headers[0].Heading != unknownHeading || headers[0].Count != 2 {
			t.Errorf("header = (%q, %d), want (%q, 2)", headers[0].Heading, headers[0].Count, unknownHeading)
		}
	})
}

func TestBuildByTag(t *testing.T) {
	t.Run("emits one item per tag for a multi-tag session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		got := map[string]SessionItem{}
		for _, si := range rows {
			if si.CatchAll {
				t.Errorf("item for tag %q is CatchAll, want false", si.GroupKey)
			}
			// For a By-Tag instance the canonical tag IS the GroupKey, and the
			// dimmed heading mirrors it.
			if si.GroupHeading != si.GroupKey {
				t.Errorf("GroupHeading = %q, want = GroupKey %q", si.GroupHeading, si.GroupKey)
			}
			got[si.GroupKey] = si
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

		// Each tag forms its own group, so one header per tag (sorted): personal
		// then work, each counting its single row.
		headers := headerRows(items)
		wantHeaders := []struct {
			heading string
			count   int
		}{
			{"personal", 1},
			{"work", 1},
		}
		if len(headers) != len(wantHeaders) {
			t.Fatalf("len(headers) = %d, want %d", len(headers), len(wantHeaders))
		}
		for i, want := range wantHeaders {
			if headers[i].Heading != want.heading || headers[i].Count != want.count {
				t.Errorf("header[%d] = (%q, %d), want (%q, %d)", i, headers[i].Heading, headers[i].Count, want.heading, want.count)
			}
		}
	})

	t.Run("treats work, Work and WORK as three distinct, case-preserved tag headings", func(t *testing.T) {
		base := t.TempDir()
		dir1 := filepath.Join(base, "one")
		dir2 := filepath.Join(base, "two")
		dir3 := filepath.Join(base, "three")
		mustMkdir(t, dir1)
		mustMkdir(t, dir2)
		mustMkdir(t, dir3)
		// Tags are case-sensitive: each case variant is its own group, displayed
		// exactly as stored.
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

		rows := sessionRows(items)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		for _, si := range rows {
			if si.GroupKey != si.GroupHeading {
				t.Errorf("GroupKey %q != GroupHeading %q (tag is its own display)", si.GroupKey, si.GroupHeading)
			}
		}
		// Sorted by (GroupKey, name): "WORK" < "Work" < "work" (ASCII order), so
		// three single-row headers in that order.
		headers := headerRows(items)
		if len(headers) != 3 {
			t.Fatalf("len(headers) = %d, want 3 (case-distinct groups)", len(headers))
		}
		want := []string{"WORK", "Work", "work"}
		for i, h := range headers {
			if h.Heading != want[i] || h.Count != 1 {
				t.Errorf("header[%d] = (%q, %d), want (%q, 1)", i, h.Heading, h.Count, want[i])
			}
		}
	})

	t.Run("emits exactly one Untagged item for a zero-tag session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "no-tags", Dir: dir}}

		items := buildByTag(sessions, project.NewIndex(projects))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Untagged" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Untagged")
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

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.GroupKey != "work" || si.GroupHeading != "work" {
			t.Errorf("item = (GroupKey %q, GroupHeading %q), want both %q", si.GroupKey, si.GroupHeading, "work")
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

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
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
			si := rows[i]
			if si.GroupKey != want.tag || si.Session.Name != want.name {
				t.Errorf("row[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, want.tag, want.name)
			}
		}

		// One header per tag group in sorted order: alpha (2 rows) then work (1).
		headers := headerRows(items)
		wantHeaders := []struct {
			heading string
			count   int
		}{
			{"alpha", 2},
			{"work", 1},
		}
		if len(headers) != len(wantHeaders) {
			t.Fatalf("len(headers) = %d, want %d", len(headers), len(wantHeaders))
		}
		for i, want := range wantHeaders {
			if headers[i].Heading != want.heading || headers[i].Count != want.count {
				t.Errorf("header[%d] = (%q, %d), want (%q, %d)", i, headers[i].Heading, headers[i].Count, want.heading, want.count)
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		first := rows[0]
		if first.CatchAll {
			t.Errorf("first row is CatchAll, want tagged item first")
		}
		last := rows[1]
		if !last.CatchAll {
			t.Errorf("last row is not CatchAll, want Untagged item last")
		}
	})

	t.Run("shares the underlying session across a multi-tag session's instances", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "personal"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir, Windows: 3, Attached: true}}

		items := buildByTag(sessions, project.NewIndex(projects))

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		a := rows[0]
		b := rows[1]
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

		rows := sessionRows(items)
		if len(rows) <= len(sessions) {
			t.Errorf("len(rows) = %d, want > live session count %d", len(rows), len(sessions))
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

		for _, si := range sessionRows(items) {
			if si.CatchAll {
				t.Errorf("unexpected catch-all item %+v; Untagged heading should be suppressed", si)
			}
			if si.GroupHeading == untaggedHeading {
				t.Errorf("unexpected Untagged heading; should be suppressed when every session is tagged")
			}
		}
		// Empty-suppression at the header layer: no Untagged HeaderItem when
		// every session is tagged.
		for _, h := range headerRows(items) {
			if h.Heading == untaggedHeading {
				t.Errorf("unexpected Untagged header; should be suppressed when every session is tagged")
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

		rows := sessionRows(items)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		wantNames := []string{"alpha", "bravo", "charlie"}
		for i, want := range wantNames {
			si := rows[i]
			if si.Session.Name != want {
				t.Errorf("row[%d].Session.Name = %q, want %q", i, si.Session.Name, want)
			}
		}
	})

	t.Run("stamps each Untagged catch-all item GroupKey with the heading constant", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByTag(sessions, project.NewIndex(nil))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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

		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		first := rows[0]
		if first.GroupHeading != "zeta" {
			t.Errorf("first row heading = %q, want %q", first.GroupHeading, "zeta")
		}
		last := rows[1]
		if !last.CatchAll || last.GroupHeading != untaggedHeading {
			t.Errorf("last row = %+v, want Untagged catch-all pinned last", last)
		}

		// The header order mirrors the row order: zeta first, Untagged pinned last.
		headers := headerRows(items)
		if len(headers) != 2 {
			t.Fatalf("len(headers) = %d, want 2", len(headers))
		}
		if headers[0].Heading != "zeta" {
			t.Errorf("first header heading = %q, want %q", headers[0].Heading, "zeta")
		}
		if headers[1].Heading != untaggedHeading {
			t.Errorf("last header heading = %q, want %q (Untagged pinned last)", headers[1].Heading, untaggedHeading)
		}
	})

	t.Run("routes a deleted-project session to Untagged", func(t *testing.T) {
		// Stamped dir, no matching project record → no tags → Untagged.
		sessions := []tmux.Session{{Name: "deleted-project", Dir: t.TempDir()}}

		items := buildByTag(sessions, project.NewIndex(nil))

		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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
		for _, si := range sessionRows(items) {
			seen[si.Session.Name] = true
		}
		for _, s := range sessions {
			if !seen[s.Name] {
				t.Errorf("session %q dropped from rendered set", s.Name)
			}
		}
	})
}
