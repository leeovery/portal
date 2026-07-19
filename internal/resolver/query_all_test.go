package resolver_test

// Tests for the K-returning multi-target resolver variants (ResolveBareAll,
// ResolveSessionPinAll, ResolveAliasPinAll). CRITICAL divergence from the single-
// result Phase-1/2 methods: in the multi-target context a not-found is a COLLECTED
// MISS (a *MissResult in the returned slice), NOT a hard error — the aggregated
// pre-flight reports EVERY unresolvable target, not just the first. These tests
// reuse the mocks declared in query_test.go (same package).

import (
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// sessionNames extracts the Name of every *SessionResult in the slice, failing the
// test on any other result type. Used to assert glob expansion order.
func sessionNames(t *testing.T, results []resolver.QueryResult) []string {
	t.Helper()
	names := make([]string, 0, len(results))
	for i, r := range results {
		sr, ok := r.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("result[%d] = %T, want *SessionResult", i, r)
		}
		names = append(names, sr.Name)
	}
	return names
}

func TestQueryResolver_ResolveBareAll(t *testing.T) {
	t.Run("session glob expands to K SessionResults with domain glob", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"api-1", "api-2", "web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := sessionNames(t, results); len(got) != 2 || got[0] != "api-1" || got[1] != "api-2" {
			t.Fatalf("names = %v, want [api-1 api-2] in order", got)
		}
		for i, r := range results {
			if sr := r.(*resolver.SessionResult); sr.Domain != "glob" {
				t.Errorf("result[%d].Domain = %q, want glob", i, sr.Domain)
			}
		}
	})

	t.Run("session glob with zero matches is a single collected miss", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "api-*" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "api-*")
		}
	})

	t.Run("non-glob exact session name is a single SessionResult", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"api-x7Kd9a"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		sr, ok := results[0].(*resolver.SessionResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *SessionResult", results[0])
		}
		if sr.Name != "api-x7Kd9a" || sr.Domain != "session" {
			t.Errorf("result = {%q, %q}, want {api-x7Kd9a, session}", sr.Name, sr.Domain)
		}
	})

	t.Run("bad path becomes a single collected miss, not a hard error", func(t *testing.T) {
		// A bare path-like value that fails ResolvePath (nonexistent dir) is a hard
		// error in the single-target Resolve; in the multi-target context it is a
		// collected miss carrying the raw target.
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("/nonexistent/path/that/does/not/exist")
		if err != nil {
			t.Fatalf("unexpected error (bad path must become a miss, not err): %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "/nonexistent/path/that/does/not/exist" {
			t.Errorf("miss.Target = %q, want the raw target", miss.Target)
		}
	})

	t.Run("gone alias dir becomes a single collected miss", func(t *testing.T) {
		// A bare value resolving via alias to a gone dir is a *DirNotFoundError hard
		// error in single-target Resolve; multi-target collapses it to a miss.
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"stale": "/gone/dir"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("stale")
		if err != nil {
			t.Fatalf("unexpected error (gone alias dir must become a miss): %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "stale" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "stale")
		}
	})

	t.Run("total miss is a single collected miss", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveBareAll("unknown")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "unknown" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "unknown")
		}
	})

	t.Run("resolved bare alias hit is a single PathResult", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"myapp": "/code/myapp"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/myapp": true}},
		)

		results, err := qr.ResolveBareAll("myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		pr, ok := results[0].(*resolver.PathResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *PathResult", results[0])
		}
		if pr.Path != "/code/myapp" || pr.Domain != "alias" {
			t.Errorf("result = {%q, %q}, want {/code/myapp, alias}", pr.Path, pr.Domain)
		}
	})
}

func TestQueryResolver_ResolveSessionPinAll(t *testing.T) {
	t.Run("glob expands to K SessionResults with domain glob", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"api-1", "api-2", "web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveSessionPinAll("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := sessionNames(t, results); len(got) != 2 || got[0] != "api-1" || got[1] != "api-2" {
			t.Fatalf("names = %v, want [api-1 api-2] in order", got)
		}
		for i, r := range results {
			if sr := r.(*resolver.SessionResult); sr.Domain != "glob" {
				t.Errorf("result[%d].Domain = %q, want glob", i, sr.Domain)
			}
		}
	})

	t.Run("exact match is a single SessionResult domain session", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"api-x7Kd9a", "web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveSessionPinAll("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		sr, ok := results[0].(*resolver.SessionResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *SessionResult", results[0])
		}
		if sr.Name != "api-x7Kd9a" || sr.Domain != "session" {
			t.Errorf("result = {%q, %q}, want {api-x7Kd9a, session}", sr.Name, sr.Domain)
		}
	})

	t.Run("exact not-found is a collected miss, NOT a hard error", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveSessionPinAll("api")
		if err != nil {
			t.Fatalf("expected a collected miss, not a hard error, got: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "api" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "api")
		}
	})

	t.Run("zero-match glob is a collected miss", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{names: []string{"web-abc"}},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveSessionPinAll("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "api-*" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "api-*")
		}
	})
}

func TestQueryResolver_ResolveAliasPinAll(t *testing.T) {
	t.Run("key glob expands to K PathResults reduced to their dirs", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{
				"workflow-a": "/code/wa",
				"workflow-b": "/code/wb",
				"blog":       "/code/blog",
			}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/wa": true, "/code/wb": true}},
		)

		results, err := qr.ResolveAliasPinAll("workflow-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len(results) = %d, want 2", len(results))
		}
		// Keys() returns sorted names, so workflow-a precedes workflow-b.
		wantDirs := []string{"/code/wa", "/code/wb"}
		for i, r := range results {
			pr, ok := r.(*resolver.PathResult)
			if !ok {
				t.Fatalf("result[%d] = %T, want *PathResult", i, r)
			}
			if pr.Path != wantDirs[i] {
				t.Errorf("result[%d].Path = %q, want %q", i, pr.Path, wantDirs[i])
			}
			if pr.Domain != "alias" {
				t.Errorf("result[%d].Domain = %q, want alias", i, pr.Domain)
			}
		}
	})

	t.Run("gone dir for one matched key becomes a miss for that key, others survive", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{
				"workflow-a": "/code/wa",
				"workflow-b": "/gone/wb",
			}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/wa": true}},
		)

		results, err := qr.ResolveAliasPinAll("workflow-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len(results) = %d, want 2", len(results))
		}
		pr, ok := results[0].(*resolver.PathResult)
		if !ok || pr.Path != "/code/wa" {
			t.Fatalf("result[0] = %#v, want PathResult /code/wa", results[0])
		}
		miss, ok := results[1].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[1] = %T, want *MissResult", results[1])
		}
		// The miss carries the KEY, not the glob value.
		if miss.Target != "workflow-b" {
			t.Errorf("miss.Target = %q, want the matched key workflow-b", miss.Target)
		}
	})

	t.Run("unknown exact key is a collected miss, NOT a hard error", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"api": "/code/api"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/api": true}},
		)

		results, err := qr.ResolveAliasPinAll("nope")
		if err != nil {
			t.Fatalf("expected a collected miss, not a hard error, got: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "nope" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "nope")
		}
	})

	t.Run("glob matching zero keys is a collected miss", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"api": "/code/api"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/api": true}},
		)

		results, err := qr.ResolveAliasPinAll("web-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "web-*" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "web-*")
		}
	})

	t.Run("known exact key with existing dir is a single PathResult", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"myapp": "/code/myapp"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{"/code/myapp": true}},
		)

		results, err := qr.ResolveAliasPinAll("myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		pr, ok := results[0].(*resolver.PathResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *PathResult", results[0])
		}
		if pr.Path != "/code/myapp" || pr.Domain != "alias" {
			t.Errorf("result = {%q, %q}, want {/code/myapp, alias}", pr.Path, pr.Domain)
		}
	})

	t.Run("gone dir for exact key is a collected miss carrying the value", func(t *testing.T) {
		qr := resolver.NewQueryResolver(
			&mockSessionLister{},
			&mockAliasLookup{aliases: map[string]string{"stale": "/gone/dir"}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)

		results, err := qr.ResolveAliasPinAll("stale")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("result[0] = %T, want *MissResult", results[0])
		}
		if miss.Target != "stale" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "stale")
		}
	})
}
