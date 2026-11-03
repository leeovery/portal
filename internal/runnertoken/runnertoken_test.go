package runnertoken_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestReadLeader(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       backend.ClientEnv
		wantToken string
	}{
		{"a runner's client leading carries its token", backend.ClientEnv{{Name: "DISPLAY", Value: ":0"}, {Name: runnertoken.Var, Value: "tok123"}}, "tok123"},
		{"a raw client leading carries none", backend.ClientEnv{{Name: "DISPLAY", Value: ":0"}}, ""},
		{"a leader that unset the token carries none", backend.ClientEnv{{Name: runnertoken.Var, Unset: true}}, ""},
		{"no client leading carries none", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
			b.SetLeaderEnv("api-x7Kd9a", tc.env)

			got, err := runnertoken.ReadLeader(t.Context(), b, "api-x7Kd9a")

			if err != nil || got.Token != tc.wantToken || !slices.Equal(got.Env, tc.env) {
				t.Errorf("ReadLeader = %+v, %v; want token %q with the leader's environment %+v", got, err, tc.wantToken, tc.env)
			}
		})
	}

	t.Run("a backend that can't say who leads is an error naming the session, never a leader", func(t *testing.T) {
		b := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		b.SetLeaderEnv("api-x7Kd9a", backend.ClientEnv{{Name: runnertoken.Var, Value: "tok123"}})
		b.Fail(backendtest.OpLeaderEnv, backend.ErrTimeout)

		got, err := runnertoken.ReadLeader(t.Context(), b, "api-x7Kd9a")

		if !errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "api-x7Kd9a") || got.Token != "" || got.Env != nil {
			t.Errorf("ReadLeader = %+v, %v; want the backend's ErrTimeout naming api-x7Kd9a, and no leader", got, err)
		}
	})
}

// The leaf lets a caller read a leader without linking the runner, so it
// reaches nothing of Portal's but the backend's vocabulary.
func TestRunnerTokenPackage_DependsOnTheBackendVocabularyAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, "github.com/leeovery/portal/internal/runnertoken",
			[]string{"github.com/leeovery/portal/internal/backend"}, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
