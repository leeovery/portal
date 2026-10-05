package resolver_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
)

func TestQueryResolver_FailedSessionListingMatchesNoSession(t *testing.T) {
	newResolver := func(t *testing.T) *resolver.QueryResolver {
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Args:   []string{"list-sessions"},
			Stderr: "no server running on /tmp/tmux-501/default",
			Err:    errors.New("exit status 1"),
		}, "list-sessions"))
		return resolver.NewQueryResolver(
			tmux.NewClient(mock),
			&mockAliasLookup{},
			&mockZoxideQuerier{err: errors.New("no match")},
			&mockDirValidator{},
		)
	}

	t.Run("the bare chain misses without an error", func(t *testing.T) {
		result, err := newResolver(t).Resolve("work")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if miss, ok := result.(*resolver.MissResult); !ok || miss.Target != "work" {
			t.Errorf("result = %#v, want a miss for work", result)
		}
	})

	t.Run("a session glob expands to a miss without an error", func(t *testing.T) {
		results, err := newResolver(t).ResolveSessionPinAll("w*")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("results = %#v, want one miss", results)
		}
		if _, ok := results[0].(*resolver.MissResult); !ok {
			t.Errorf("result = %#v, want a miss", results[0])
		}
	})
}
