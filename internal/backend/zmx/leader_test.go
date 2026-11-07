package zmx_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestLeaderEnv(t *testing.T) {
	for _, tt := range []struct {
		name    string
		answer  [][]byte
		want    backend.ClientEnv
		payload string
	}{
		{
			name:   "it reads the leader's variables, set and unset, as EnvData lists them",
			answer: [][]byte{frame(21, 1, []byte("A=1\n-B\nC=x=y\nD=\n"))},
			want:   backend.ClientEnv{{Name: "A", Value: "1"}, {Name: "B", Unset: true}, {Name: "C", Value: "x=y"}, {Name: "D"}},
		},
		{
			name:   "it skips the session's output before the answer",
			answer: [][]byte{frame(1, 1, []byte("output")), frame(21, 1, []byte("A=1\n"))},
			want:   backend.ClientEnv{{Name: "A", Value: "1"}},
		},
		{
			name:   "no client leading is no environment",
			answer: [][]byte{frame(21, 1, nil)},
		},
		{
			name:   "a line of neither form is skipped",
			answer: [][]byte{frame(21, 1, []byte("stray\n=nameless\n-\nA=1"))},
			want:   backend.ClientEnv{{Name: "A", Value: "1"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := socketDir(t)
			d := serve(t, dir, "work", answering(tt.answer...))

			env, err := newBackend(t, dir).LeaderEnv(t.Context(), "work")

			if err != nil || !slices.Equal(env, tt.want) {
				t.Errorf("LeaderEnv = %+v, %v; want %+v", env, err, tt.want)
			}
			if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 19 || reqs[0].version != 1 || len(reqs[0].payload) != 0 {
				t.Errorf("the daemon received %+v, want one empty EnvGet", reqs)
			}
		})
	}

	t.Run("a daemon that does not know the request says so, which is no timeout", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", answering(frame(255, 1, nil)))

		_, err := newBackend(t, dir).LeaderEnv(t.Context(), "work")

		if err == nil || errors.Is(err, backend.ErrTimeout) {
			t.Errorf("LeaderEnv = %v, want the refusal, not ErrTimeout", err)
		}
	})
}

func TestTrack(t *testing.T) {
	tracking := func(list string) string { return "ZMX_TRACK_ENV=" + list }

	for _, tt := range []struct {
		name string
		env  []string
		want []string
	}{
		{
			name: "with no list of its own, the client tracks zmx's default list and the variable",
			env:  []string{"PATH=/bin"},
			want: []string{"PATH=/bin", tracking(zmx.DefaultTracked + ",PORTAL_ATTACH"), "PORTAL_ATTACH=tok123"},
		},
		{
			name: "a list of its own is kept, the variable added",
			env:  []string{tracking("DISPLAY,FOO")},
			want: []string{tracking("DISPLAY,FOO,PORTAL_ATTACH"), "PORTAL_ATTACH=tok123"},
		},
		{
			name: "a list naming the variable already, as zmx trims it, is kept as it is",
			env:  []string{tracking(" PORTAL_ATTACH ,FOO")},
			want: []string{tracking(" PORTAL_ATTACH ,FOO"), "PORTAL_ATTACH=tok123"},
		},
		{
			name: "a list set empty tracks nothing, so the variable alone",
			env:  []string{tracking("")},
			want: []string{tracking("PORTAL_ATTACH"), "PORTAL_ATTACH=tok123"},
		},
		{
			name: "a value already set is replaced",
			env:  []string{"PORTAL_ATTACH=old", tracking("PORTAL_ATTACH"), "PORTAL_ATTACH=older"},
			want: []string{tracking("PORTAL_ATTACH"), "PORTAL_ATTACH=tok123"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBackend(t, socketDir(t))
			given := backend.Command{Argv: []string{"zmx", "attach", "work"}, Env: slices.Clone(tt.env)}

			got := b.Track(given, "PORTAL_ATTACH", "tok123")

			if !slices.Equal(got.Env, tt.want) {
				t.Errorf("Track Env = %q, want %q", got.Env, tt.want)
			}
			if !slices.Equal(given.Env, tt.env) {
				t.Errorf("the given Env became %q, want it left %q", given.Env, tt.env)
			}
		})
	}
}
