package resolver_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// mockKeptLookup keeps the names it holds, or fails every ask with err, and
// records each name asked about.
type mockKeptLookup struct {
	kept  []string
	err   error
	asked []string
}

func (m *mockKeptLookup) IsKept(name string) (bool, error) {
	m.asked = append(m.asked, name)
	return slices.Contains(m.kept, name), m.err
}

// keptResolver is a resolver over the live sessions and the kept ones, with
// an alias and a zoxide match for every name it is asked about, a directory
// that exists for each: whatever reaches them resolves to a path.
func keptResolver(live []string, kept resolver.KeptLookup) *resolver.QueryResolver {
	aliases := map[string]string{}
	for _, name := range []string{"work", ".notes", "web"} {
		aliases[name] = "/aliased/" + name
	}
	return resolver.NewQueryResolver(
		&mockSessionLister{names: live},
		&mockAliasLookup{aliases: aliases},
		&mockZoxideQuerier{result: "/zoxide/match"},
		&mockDirValidator{existing: map[string]bool{"/aliased/work": true, "/aliased/.notes": true, "/aliased/web": true, "/zoxide/match": true}},
	).WithKept(kept)
}

func assertKept(t *testing.T, got resolver.QueryResult, err error, name string) {
	t.Helper()
	if err != nil {
		t.Fatalf("err = %v, want a kept result", err)
	}
	kept, ok := got.(*resolver.KeptResult)
	if !ok || kept.Name != name {
		t.Fatalf("result = %#v, want *KeptResult{Name: %q}", got, name)
	}
}

func assertSession(t *testing.T, got resolver.QueryResult, err error, name string) {
	t.Helper()
	if err != nil {
		t.Fatalf("err = %v, want a session result", err)
	}
	s, ok := got.(*resolver.SessionResult)
	if !ok || s.Name != name || s.Domain != resolver.DomainSession {
		t.Fatalf("result = %#v, want *SessionResult{Name: %q, Domain: session}", got, name)
	}
}

func TestResolve_KeptSavedSession(t *testing.T) {
	t.Run("an exact live session comes first, the kept one never asked about", func(t *testing.T) {
		kept := &mockKeptLookup{kept: []string{"work"}}

		got, err := keptResolver([]string{"work"}, kept).Resolve("work")

		assertSession(t, got, err, "work")
		if len(kept.asked) != 0 {
			t.Errorf("asked about %q, want nothing asked once a live session holds the name", kept.asked)
		}
	})

	t.Run("a kept name resolves before an alias and zoxide", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{kept: []string{"work"}}).Resolve("work")

		assertKept(t, got, err, "work")
	})

	t.Run("a kept name resolves before a path", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{kept: []string{".notes"}}).Resolve(".notes")

		assertKept(t, got, err, ".notes")
	})

	t.Run("a name not kept falls through to the alias", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{}).Resolve("work")

		if p, ok := got.(*resolver.PathResult); err != nil || !ok || p.Domain != resolver.DomainAlias {
			t.Errorf("Resolve = %#v, %v; want the alias's path", got, err)
		}
	})

	t.Run("a lookup that fails keeps nothing, and resolution goes on", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{kept: []string{"work"}, err: errors.New("sessions.json unreadable")}).Resolve("work")

		if p, ok := got.(*resolver.PathResult); err != nil || !ok || p.Domain != resolver.DomainAlias {
			t.Errorf("Resolve = %#v, %v; want the alias's path", got, err)
		}
	})

	t.Run("a resolver given no lookup keeps nothing", func(t *testing.T) {
		got, err := keptResolver(nil, nil).Resolve("work")

		if _, ok := got.(*resolver.PathResult); err != nil || !ok {
			t.Errorf("Resolve = %#v, %v; want the alias's path", got, err)
		}
	})

	t.Run("WithKept leaves the resolver it copies as it was", func(t *testing.T) {
		kept := &mockKeptLookup{kept: []string{"work"}}
		qr := keptResolver(nil, kept)

		without := qr.WithKept(nil)

		if got, err := without.Resolve("work"); err != nil || slices.Contains(kept.asked, "work") {
			t.Errorf("the copy resolved %#v, %v, asking %q; want nothing kept", got, err, kept.asked)
		}
		got, err := qr.Resolve("work")
		assertKept(t, got, err, "work")
	})
}

func TestResolveSessionPin_KeptSavedSession(t *testing.T) {
	t.Run("an exact live session comes first", func(t *testing.T) {
		got, err := keptResolver([]string{"work"}, &mockKeptLookup{kept: []string{"work"}}).ResolveSessionPin("work")

		assertSession(t, got, err, "work")
	})

	t.Run("a kept name resolves", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{kept: []string{"work"}}).ResolveSessionPin("work")

		assertKept(t, got, err, "work")
	})

	t.Run("a name neither live nor kept is no session", func(t *testing.T) {
		got, err := keptResolver(nil, &mockKeptLookup{}).ResolveSessionPin("work")

		if err == nil || err.Error() != "No session found: work" {
			t.Errorf("ResolveSessionPin = %#v, %v; want No session found: work", got, err)
		}
	})
}

func TestResolveAll_KeptSavedSession(t *testing.T) {
	t.Run("a bare exact name in a set resolves kept", func(t *testing.T) {
		got, _ := keptResolver(nil, &mockKeptLookup{kept: []string{"work"}}).ResolveBareAll("work")

		if len(got) != 1 {
			t.Fatalf("ResolveBareAll = %#v, want one result", got)
		}
		assertKept(t, got[0], nil, "work")
	})

	t.Run("a pinned exact name in a set resolves kept", func(t *testing.T) {
		got, _ := keptResolver(nil, &mockKeptLookup{kept: []string{"work"}}).ResolveSessionPinAll("work")

		if len(got) != 1 {
			t.Fatalf("ResolveSessionPinAll = %#v, want one result", got)
		}
		assertKept(t, got[0], nil, "work")
	})

	for name, resolve := range map[string]func(*resolver.QueryResolver, string) ([]resolver.QueryResult, error){
		"bare":   (*resolver.QueryResolver).ResolveBareAll,
		"pinned": (*resolver.QueryResolver).ResolveSessionPinAll,
	} {
		t.Run("a "+name+" glob expands over live sessions alone", func(t *testing.T) {
			kept := &mockKeptLookup{kept: []string{"web", "work"}}

			got, _ := resolve(keptResolver([]string{"wiki"}, kept), "w*")

			if len(got) != 1 {
				t.Fatalf("%s glob = %#v, want the live session alone", name, got)
			}
			assertSessionGlob(t, got[0], "wiki")
			if len(kept.asked) != 0 {
				t.Errorf("asked about %q, want a glob never to ask", kept.asked)
			}
		})

		t.Run("a "+name+" glob matching only kept names misses", func(t *testing.T) {
			got, _ := resolve(keptResolver(nil, &mockKeptLookup{kept: []string{"web", "work"}}), "w*")

			if len(got) != 1 {
				t.Fatalf("%s glob = %#v, want one miss", name, got)
			}
			if miss, ok := got[0].(*resolver.MissResult); !ok || miss.Target != "w*" {
				t.Errorf("%s glob = %#v, want a miss on w*", name, got[0])
			}
		})
	}
}

func assertSessionGlob(t *testing.T, got resolver.QueryResult, name string) {
	t.Helper()
	s, ok := got.(*resolver.SessionResult)
	if !ok || s.Name != name || s.Domain != resolver.DomainGlob {
		t.Errorf("result = %#v, want *SessionResult{Name: %q, Domain: glob}", got, name)
	}
}
